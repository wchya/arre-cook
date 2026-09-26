package services

import (
	"errors"
	"fmt"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 菜谱删除规则：个人私房菜本人直接删；家庭共享菜谱只有家庭创建者（管理员）能直接删，
// 其他家庭成员提交删除申请，由管理员同意后才删除；公共菜谱仅站点管理员可删。

const (
	DeleteModeDirect  = "direct"
	DeleteModeRequest = "request"
	DeleteModeNone    = "none"

	dishRequestPending   = "pending"
	dishRequestApproved  = "approved"
	dishRequestRejected  = "rejected"
	dishRequestCancelled = "cancelled"
)

var ErrDishRequestNotFound = errors.New("删除申请不存在或已处理")

// DishAccessFor 计算 uid 对这道菜的编辑 / 删除权限；isAdmin 表示站点管理员。
func DishAccessFor(uid uint, isAdmin bool, d *models.Dish) models.DishAccess {
	access := models.DishAccess{DeleteMode: DeleteModeNone}
	switch {
	case d.FamilyID != 0:
		family, err := FamilyForUser(uid)
		if err != nil || family == nil || family.ID != d.FamilyID {
			return access
		}
		isOwner := family.OwnerID == uid
		access.CanEdit = isOwner || d.OwnerID == uid
		access.DeleteMode = DeleteModeRequest
		if isOwner {
			access.DeleteMode = DeleteModeDirect
		}
		access.PendingRequestID = pendingDishRequestID(d.ID)
	case d.OwnerID == 0:
		if isAdmin {
			access.CanEdit, access.DeleteMode = true, DeleteModeDirect
		}
	case d.OwnerID == uid:
		access.CanEdit, access.DeleteMode = true, DeleteModeDirect
	}
	return access
}

func pendingDishRequestID(dishID uint) uint {
	var req models.DishDeleteRequest
	if err := database.DB.Select("id").Where("dish_id = ? AND status = ?", dishID, dishRequestPending).First(&req).Error; err != nil {
		return 0
	}
	return req.ID
}

// DeleteDishCascade 删除菜品，并清理引用它的家庭菜单格、家庭自动买菜条目与待处理的删除申请，
// 让受影响用户的周菜单下次重新生成。actor 为执行删除的用户。
func DeleteDishCascade(d *models.Dish, actor uint) error {
	var closed []models.DishDeleteRequest
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(d).Error; err != nil {
			return err
		}
		if d.FamilyID == 0 {
			return nil
		}
		if err := tx.Where("family_id = ? AND dish_id = ?", d.FamilyID, d.ID).Delete(&models.FamilyPlanItem{}).Error; err != nil {
			return err
		}
		if err := tx.Where("family_id = ? AND dish_id = ?", d.FamilyID, d.ID).Delete(&models.FamilyShoppingCheck{}).Error; err != nil {
			return err
		}
		if err := tx.Where("dish_id = ? AND status = ?", d.ID, dishRequestPending).Find(&closed).Error; err != nil {
			return err
		}
		if len(closed) == 0 {
			return nil
		}
		return tx.Model(&models.DishDeleteRequest{}).Where("dish_id = ? AND status = ?", d.ID, dishRequestPending).
			Updates(map[string]any{"status": dishRequestApproved, "decided_by": actor, "decided_at": time.Now()}).Error
	})
	if err != nil {
		return err
	}
	InvalidateWeekPlan(actor)
	if d.FamilyID != 0 {
		for _, uid := range familyMemberUserIDs(d.FamilyID) {
			if uid != actor {
				InvalidateWeekPlan(uid)
			}
		}
	}
	// 管理员直接删掉了有人申请删除的菜：通知申请人，免得对方一直等审核结果
	for _, req := range closed {
		if req.RequestedBy != actor {
			_, _ = CreateNotification(req.RequestedBy, "family", "删除申请已完成",
				fmt.Sprintf("你申请删除的家庭菜谱「%s」已被家庭管理员删除。", req.DishName), "/family?tab=requests")
		}
	}
	return nil
}

// DishDeleteRequestView 删除申请的展示视图（附菜品封面与双方昵称）。
type DishDeleteRequestView struct {
	ID            uint       `json:"id"`
	DishID        uint       `json:"dish_id"`
	DishName      string     `json:"dish_name"`
	DishImage     string     `json:"dish_image"`
	RequestedBy   uint       `json:"requested_by"`
	RequesterName string     `json:"requester_name"`
	Status        string     `json:"status"`
	Reason        string     `json:"reason"`
	DecidedBy     uint       `json:"decided_by"`
	DeciderName   string     `json:"decider_name"`
	DecidedAt     *time.Time `json:"decided_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

func dishRequestViews(rows []models.DishDeleteRequest) []DishDeleteRequestView {
	out := make([]DishDeleteRequestView, 0, len(rows))
	if len(rows) == 0 {
		return out
	}
	userIDs := make([]uint, 0, len(rows)*2)
	dishIDs := make([]uint, 0, len(rows))
	for _, r := range rows {
		userIDs = append(userIDs, r.RequestedBy)
		if r.DecidedBy != 0 {
			userIDs = append(userIDs, r.DecidedBy)
		}
		dishIDs = append(dishIDs, r.DishID)
	}
	names := userDisplayNames(userIDs)
	images := map[uint]string{}
	var dishes []models.Dish
	// 已删除的菜也要能取到封面，用于展示已通过的申请
	database.DB.Unscoped().Select("id", "image_url", "images").Where("id IN ?", dishIDs).Find(&dishes)
	for _, d := range dishes {
		images[d.ID] = DishImageURL(d)
	}
	for _, r := range rows {
		out = append(out, DishDeleteRequestView{
			ID: r.ID, DishID: r.DishID, DishName: r.DishName, DishImage: images[r.DishID],
			RequestedBy: r.RequestedBy, RequesterName: names[r.RequestedBy], Status: r.Status, Reason: r.Reason,
			DecidedBy: r.DecidedBy, DeciderName: names[r.DecidedBy], DecidedAt: r.DecidedAt, CreatedAt: r.CreatedAt,
		})
	}
	return out
}

func dishRequestView(r models.DishDeleteRequest) *DishDeleteRequestView {
	view := dishRequestViews([]models.DishDeleteRequest{r})[0]
	return &view
}

// RequestDishDeletion 家庭成员申请删除家庭共享菜谱并通知家庭管理员；
// 这道菜已有待处理申请时直接返回那一条（幂等）。
func RequestDishDeletion(uid uint, d *models.Dish) (*DishDeleteRequestView, error) {
	family, err := RequireFamily(uid)
	if err != nil {
		return nil, err
	}
	if d.FamilyID == 0 || d.FamilyID != family.ID {
		return nil, ErrDishNotFound
	}
	var existing models.DishDeleteRequest
	if err := database.DB.Where("dish_id = ? AND status = ?", d.ID, dishRequestPending).First(&existing).Error; err == nil {
		return dishRequestView(existing), nil
	}
	req := models.DishDeleteRequest{FamilyID: family.ID, DishID: d.ID, DishName: d.Name, RequestedBy: uid, Status: dishRequestPending}
	if err := database.DB.Create(&req).Error; err != nil {
		return nil, err
	}
	if family.OwnerID != uid {
		_, _ = CreateNotification(family.OwnerID, "family", "菜谱删除申请",
			fmt.Sprintf("%s 申请删除家庭共享菜谱「%s」，同意后才会真正删除。", userDisplayName(uid), d.Name), "/family?tab=requests")
	}
	return dishRequestView(req), nil
}

// ListDishRequests 家庭管理员看到全家的申请，其他成员只看到自己提交的。status 为 all 时含已处理的。
func ListDishRequests(uid uint, status string) ([]DishDeleteRequestView, error) {
	family, err := RequireFamily(uid)
	if err != nil {
		return nil, err
	}
	q := database.DB.Where("family_id = ?", family.ID)
	if family.OwnerID != uid {
		q = q.Where("requested_by = ?", uid)
	}
	if strings.TrimSpace(status) != "all" {
		q = q.Where("status = ?", dishRequestPending)
	}
	var rows []models.DishDeleteRequest
	if err := q.Order("created_at DESC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	return dishRequestViews(rows), nil
}

// PendingDishRequestCount 家庭页角标：管理员计全家待处理，成员计自己待处理的。
func PendingDishRequestCount(uid uint, family *models.Family) int64 {
	if family == nil {
		return 0
	}
	q := database.DB.Model(&models.DishDeleteRequest{}).Where("family_id = ? AND status = ?", family.ID, dishRequestPending)
	if family.OwnerID != uid {
		q = q.Where("requested_by = ?", uid)
	}
	var n int64
	q.Count(&n)
	return n
}

func findPendingFamilyRequest(familyID, id uint) (*models.DishDeleteRequest, error) {
	var req models.DishDeleteRequest
	if err := database.DB.Where("id = ? AND family_id = ? AND status = ?", id, familyID, dishRequestPending).First(&req).Error; err != nil {
		return nil, ErrDishRequestNotFound
	}
	return &req, nil
}

// ApproveDishRequest 家庭管理员同意删除：删除菜品（连带家庭菜单与自动买菜条目）并通知申请人。
func ApproveDishRequest(uid, id uint) (*DishDeleteRequestView, error) {
	family, err := requireFamilyOwner(uid)
	if err != nil {
		return nil, err
	}
	req, err := findPendingFamilyRequest(family.ID, id)
	if err != nil {
		return nil, err
	}
	var dish models.Dish
	if err := database.DB.Where("id = ? AND family_id = ?", req.DishID, family.ID).First(&dish).Error; err == nil {
		if err := DeleteDishCascade(&dish, uid); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	if err := database.DB.Model(req).Updates(map[string]any{"status": dishRequestApproved, "decided_by": uid, "decided_at": now}).Error; err != nil {
		return nil, err
	}
	req.Status, req.DecidedBy, req.DecidedAt = dishRequestApproved, uid, &now
	if req.RequestedBy != uid {
		_, _ = CreateNotification(req.RequestedBy, "family", "删除申请已通过",
			fmt.Sprintf("家庭管理员同意了你的申请，「%s」已从家庭菜谱中删除。", req.DishName), "/family?tab=requests")
	}
	return dishRequestView(*req), nil
}

// RejectDishRequest 家庭管理员拒绝删除，可附理由，通知申请人。
func RejectDishRequest(uid, id uint, reason string) (*DishDeleteRequestView, error) {
	family, err := requireFamilyOwner(uid)
	if err != nil {
		return nil, err
	}
	req, err := findPendingFamilyRequest(family.ID, id)
	if err != nil {
		return nil, err
	}
	reason = truncateRunes(strings.TrimSpace(reason), 100)
	now := time.Now()
	if err := database.DB.Model(req).Updates(map[string]any{"status": dishRequestRejected, "reason": reason, "decided_by": uid, "decided_at": now}).Error; err != nil {
		return nil, err
	}
	req.Status, req.Reason, req.DecidedBy, req.DecidedAt = dishRequestRejected, reason, uid, &now
	if req.RequestedBy != uid {
		content := fmt.Sprintf("家庭管理员保留了「%s」。", req.DishName)
		if reason != "" {
			content += "理由：" + reason
		}
		_, _ = CreateNotification(req.RequestedBy, "family", "删除申请未通过", content, "/family?tab=requests")
	}
	return dishRequestView(*req), nil
}

// CancelDishRequest 申请人撤回自己仍在等待处理的删除申请。
func CancelDishRequest(uid, id uint) error {
	res := database.DB.Model(&models.DishDeleteRequest{}).
		Where("id = ? AND requested_by = ? AND status = ?", id, uid, dishRequestPending).
		Updates(map[string]any{"status": dishRequestCancelled, "decided_by": uid, "decided_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrDishRequestNotFound
	}
	return nil
}

func familyMemberUserIDs(familyID uint) []uint {
	var ids []uint
	database.DB.Model(&models.FamilyMember{}).Where("family_id = ?", familyID).Pluck("user_id", &ids)
	return ids
}

func userDisplayNames(ids []uint) map[uint]string {
	out := make(map[uint]string, len(ids))
	if len(ids) == 0 {
		return out
	}
	var users []models.User
	database.DB.Where("id IN ?", ids).Find(&users)
	for _, u := range users {
		out[u.ID] = u.DisplayName()
	}
	return out
}

func userDisplayName(uid uint) string {
	if name := userDisplayNames([]uint{uid})[uid]; name != "" {
		return name
	}
	return "家庭成员"
}
