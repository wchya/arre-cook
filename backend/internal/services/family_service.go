package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrFamilyRequired  = errors.New("请先创建或加入家庭")
	ErrFamilyOwnerOnly = errors.New("只有家庭创建者可以操作")
	ErrFamilyConflict  = errors.New("你已经加入一个家庭")
	ErrFamilyLimit     = errors.New("家庭成员或邀请已达上限")
	ErrInviteInvalid   = errors.New("邀请无效或已过期")
	ErrInviteEmail     = errors.New("请使用受邀的邮箱账号登录")
	ErrFamilyDish      = errors.New("只能把公共或家庭共享菜谱加入家庭菜单")
)

const (
	familyMemberLimit = 12
	familyInviteTTL   = 7 * 24 * time.Hour
)

type FamilyMemberView struct {
	UserID   uint      `json:"user_id"`
	Nickname string    `json:"nickname"`
	Email    string    `json:"email"`
	Avatar   string    `json:"avatar"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type FamilySnapshot struct {
	Family      *models.Family            `json:"family"`
	Role        string                    `json:"role"`
	Members     []FamilyMemberView        `json:"members"`
	Invitations []models.FamilyInvitation `json:"invitations"`
}

func FamilyForUser(uid uint) (*models.Family, error) {
	var member models.FamilyMember
	if err := database.DB.Where("user_id = ?", uid).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var family models.Family
	if err := database.DB.First(&family, member.FamilyID).Error; err != nil {
		return nil, err
	}
	return &family, nil
}

func RequireFamily(uid uint) (*models.Family, error) {
	family, err := FamilyForUser(uid)
	if err != nil {
		return nil, err
	}
	if family == nil {
		return nil, ErrFamilyRequired
	}
	return family, nil
}

func requireFamilyOwner(uid uint) (*models.Family, error) {
	family, err := RequireFamily(uid)
	if err != nil {
		return nil, err
	}
	if family.OwnerID != uid {
		return nil, ErrFamilyOwnerOnly
	}
	return family, nil
}

func FamilyInfo(uid uint) (FamilySnapshot, error) {
	family, err := FamilyForUser(uid)
	empty := FamilySnapshot{Members: []FamilyMemberView{}, Invitations: []models.FamilyInvitation{}}
	if err != nil || family == nil {
		return empty, err
	}
	empty.Family = family
	empty.Role = "member"
	if family.OwnerID == uid {
		empty.Role = "owner"
	}
	var members []models.FamilyMember
	if err := database.DB.Where("family_id = ?", family.ID).Order("joined_at ASC").Find(&members).Error; err != nil {
		return empty, err
	}
	ids := make([]uint, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	var users []models.User
	if err := database.DB.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return empty, err
	}
	byID := make(map[uint]models.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	for _, m := range members {
		u, ok := byID[m.UserID]
		if !ok {
			continue
		}
		v := FamilyMemberView{UserID: u.ID, Nickname: u.DisplayName(), Avatar: u.Avatar, Role: "member", JoinedAt: m.JoinedAt}
		if u.Email != nil {
			v.Email = *u.Email
		}
		if u.ID == family.OwnerID {
			v.Role = "owner"
		}
		empty.Members = append(empty.Members, v)
	}
	if empty.Role == "owner" {
		err = database.DB.Where("family_id = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?", family.ID, time.Now()).
			Order("created_at DESC").Find(&empty.Invitations).Error
	}
	return empty, err
}

func CreateFamily(uid uint, name string) (*models.Family, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 30 {
		return nil, errors.New("家庭名称请填写 1-30 个字")
	}
	family := &models.Family{Name: name, OwnerID: uid}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&models.FamilyMember{}).Where("user_id = ?", uid).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrFamilyConflict
		}
		if err := tx.Create(family).Error; err != nil {
			return err
		}
		return tx.Create(&models.FamilyMember{FamilyID: family.ID, UserID: uid, JoinedAt: time.Now()}).Error
	})
	return family, err
}

func RenameFamily(uid uint, name string) error {
	family, err := requireFamilyOwner(uid)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 30 {
		return errors.New("家庭名称请填写 1-30 个字")
	}
	return database.DB.Model(family).Update("name", name).Error
}

func inviteHash(raw string) string {
	sum := sha256.Sum256([]byte(raw + "|" + config.C.JWTSecret))
	return hex.EncodeToString(sum[:])
}

func CreateFamilyInvitation(uid uint, rawEmail string) (*models.FamilyInvitation, string, error) {
	family, err := requireFamilyOwner(uid)
	if err != nil {
		return nil, "", err
	}
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return nil, "", err
	}
	var recipient models.User
	if err := database.DB.Where("email = ?", email).First(&recipient).Error; err == nil {
		var n int64
		database.DB.Model(&models.FamilyMember{}).Where("user_id = ? AND family_id = ?", recipient.ID, family.ID).Count(&n)
		if n > 0 {
			return nil, "", errors.New("这位用户已经是家庭成员")
		}
	}
	now := time.Now()
	var count int64
	database.DB.Model(&models.FamilyInvitation{}).Where("family_id = ? AND created_at > ?", family.ID, now.Add(-24*time.Hour)).Count(&count)
	if count >= 30 {
		return nil, "", errors.New("今天的邀请次数已达上限")
	}
	database.DB.Model(&models.FamilyInvitation{}).Where("family_id = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?", family.ID, now).Count(&count)
	if count >= familyMemberLimit {
		return nil, "", ErrFamilyLimit
	}
	var last models.FamilyInvitation
	if err := database.DB.Where("family_id = ? AND email = ?", family.ID, email).Order("created_at DESC").First(&last).Error; err == nil && now.Sub(last.CreatedAt) < time.Minute {
		return nil, "", errors.New("请稍后再向该邮箱发送邀请")
	}
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return nil, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	invite := &models.FamilyInvitation{FamilyID: family.ID, Email: email, TokenHash: inviteHash(token), InvitedBy: uid, ExpiresAt: now.Add(familyInviteTTL)}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.FamilyInvitation{}).
			Where("family_id = ? AND email = ? AND accepted_at IS NULL AND revoked_at IS NULL", family.ID, email).
			Update("revoked_at", now).Error; err != nil {
			return err
		}
		return tx.Create(invite).Error
	})
	if err == nil && recipient.ID != 0 {
		_, _ = CreateNotification(recipient.ID, "family", "收到家庭邀请", "有人邀请你加入「"+family.Name+"」，打开家庭页面即可查看邀请。", "/family")
	}
	return invite, token, err
}

func RevokeFamilyInvitation(uid, id uint) error {
	family, err := requireFamilyOwner(uid)
	if err != nil {
		return err
	}
	now := time.Now()
	res := database.DB.Model(&models.FamilyInvitation{}).
		Where("id = ? AND family_id = ? AND accepted_at IS NULL AND revoked_at IS NULL", id, family.ID).
		Update("revoked_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrInviteInvalid
	}
	return nil
}

func JoinFamily(uid uint, token string) (*models.Family, error) {
	token = strings.TrimSpace(token)
	if len(token) != 32 {
		return nil, ErrInviteInvalid
	}
	var user models.User
	if err := database.DB.First(&user, uid).Error; err != nil || user.Email == nil {
		return nil, ErrInviteEmail
	}
	var joined models.Family
	now := time.Now()
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var invite models.FamilyInvitation
		if err := tx.Where("token_hash = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?", inviteHash(token), now).First(&invite).Error; err != nil {
			return ErrInviteInvalid
		}
		if invite.Email != *user.Email {
			return ErrInviteEmail
		}
		var n int64
		if err := tx.Model(&models.FamilyMember{}).Where("user_id = ?", uid).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrFamilyConflict
		}
		if err := tx.Model(&models.FamilyMember{}).Where("family_id = ?", invite.FamilyID).Count(&n).Error; err != nil {
			return err
		}
		if n >= familyMemberLimit {
			return ErrFamilyLimit
		}
		if err := tx.First(&joined, invite.FamilyID).Error; err != nil {
			return ErrInviteInvalid
		}
		res := tx.Model(&models.FamilyInvitation{}).Where("id = ? AND accepted_at IS NULL AND revoked_at IS NULL", invite.ID).Update("accepted_at", now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrInviteInvalid
		}
		return tx.Create(&models.FamilyMember{FamilyID: invite.FamilyID, UserID: uid, JoinedAt: now}).Error
	})
	if err == nil && joined.OwnerID != uid {
		_, _ = CreateNotification(joined.OwnerID, "family", "家庭成员已加入", "新的家庭成员已经接受邀请，家庭菜谱和菜单可以一起管理了。", "/family")
	}
	return &joined, err
}

func TransferFamily(uid, nextOwner uint) error {
	family, err := requireFamilyOwner(uid)
	if err != nil {
		return err
	}
	if nextOwner == uid {
		return nil
	}
	var n int64
	if err := database.DB.Model(&models.FamilyMember{}).Where("family_id = ? AND user_id = ?", family.ID, nextOwner).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return errors.New("请选择当前家庭成员")
	}
	return database.DB.Model(family).Update("owner_id", nextOwner).Error
}

func RemoveFamilyMember(uid, target uint) error {
	family, err := requireFamilyOwner(uid)
	if err != nil {
		return err
	}
	if target == uid {
		return errors.New("创建者不能移除自己，请先转让家庭")
	}
	res := database.DB.Where("family_id = ? AND user_id = ?", family.ID, target).Delete(&models.FamilyMember{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("成员不存在")
	}
	InvalidateWeekPlan(target)
	return nil
}

func LeaveFamily(uid uint) error {
	family, err := RequireFamily(uid)
	if err != nil {
		return err
	}
	if family.OwnerID == uid {
		return errors.New("创建者请先转让家庭，或解散家庭")
	}
	if err := database.DB.Where("family_id = ? AND user_id = ?", family.ID, uid).Delete(&models.FamilyMember{}).Error; err != nil {
		return err
	}
	InvalidateWeekPlan(uid)
	return nil
}

func DeleteFamily(uid uint) error {
	family, err := requireFamilyOwner(uid)
	if err != nil {
		return err
	}
	var members []models.FamilyMember
	if err := database.DB.Where("family_id = ?", family.ID).Find(&members).Error; err != nil {
		return err
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&models.FamilyInvitation{}, &models.FamilyPlanItem{}, &models.FamilyShoppingItem{}, &models.FamilyMember{}} {
			if err := tx.Where("family_id = ?", family.ID).Delete(model).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("family_id = ?", family.ID).Delete(&models.Dish{}).Error; err != nil {
			return err
		}
		return tx.Delete(family).Error
	})
	if err == nil {
		for _, m := range members {
			InvalidateWeekPlan(m.UserID)
		}
	}
	return err
}

type FamilyPlanView struct {
	ID       uint        `json:"id"`
	MealDate string      `json:"meal_date"`
	MealType string      `json:"meal_type"`
	Dish     models.Dish `json:"dish"`
	AddedBy  uint        `json:"added_by"`
}

func FamilyPlan(uid uint, start string) ([]FamilyPlanView, error) {
	family, err := RequireFamily(uid)
	if err != nil {
		return nil, err
	}
	if start == "" {
		start = Today()
	}
	date, err := time.Parse("2006-01-02", start)
	if err != nil {
		return nil, ErrInvalidDate
	}
	end := date.AddDate(0, 0, 7).Format("2006-01-02")
	var items []models.FamilyPlanItem
	if err := database.DB.Where("family_id = ? AND meal_date >= ? AND meal_date < ?", family.ID, start, end).
		Order("meal_date ASC, meal_type ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	out := make([]FamilyPlanView, 0, len(items))
	for _, item := range items {
		var dish models.Dish
		if err := database.DB.Scopes(database.VisibleDishes(uid)).First(&dish, item.DishID).Error; err == nil {
			out = append(out, FamilyPlanView{ID: item.ID, MealDate: item.MealDate, MealType: item.MealType, Dish: dish, AddedBy: item.AddedBy})
		}
	}
	return out, nil
}

func SetFamilyPlan(uid uint, rawDate, mealType string, dishID uint) error {
	family, err := RequireFamily(uid)
	if err != nil {
		return err
	}
	date, err := time.Parse("2006-01-02", rawDate)
	if err != nil || date.Format("2006-01-02") != rawDate {
		return ErrInvalidDate
	}
	today, _ := time.Parse("2006-01-02", Today())
	if date.Before(today.AddDate(0, 0, -7)) || date.After(today.AddDate(0, 0, 30)) {
		return errors.New("家庭菜单只能安排最近 7 天到未来 30 天")
	}
	if mealType != "lunch" && mealType != "dinner" {
		return ErrInvalidMealType
	}
	if dishID == 0 {
		return database.DB.Where("family_id = ? AND meal_date = ? AND meal_type = ?", family.ID, rawDate, mealType).
			Delete(&models.FamilyPlanItem{}).Error
	}
	var dish models.Dish
	if err := database.DB.Where("id = ? AND enabled = ? AND ((owner_id = 0 AND family_id = 0) OR family_id = ?)", dishID, true, family.ID).First(&dish).Error; err != nil {
		return ErrFamilyDish
	}
	item := models.FamilyPlanItem{FamilyID: family.ID, MealDate: rawDate, MealType: mealType, DishID: dishID, AddedBy: uid}
	return database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "family_id"}, {Name: "meal_date"}, {Name: "meal_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"dish_id", "added_by", "updated_at"}),
	}).Create(&item).Error
}

func FamilyShopping(uid uint) ([]models.FamilyShoppingItem, error) {
	family, err := RequireFamily(uid)
	if err != nil {
		return nil, err
	}
	items := []models.FamilyShoppingItem{}
	err = database.DB.Where("family_id = ?", family.ID).Order("checked ASC, created_at DESC").Find(&items).Error
	return items, err
}

func AddFamilyShopping(uid uint, name, amount string) error {
	family, err := RequireFamily(uid)
	if err != nil {
		return err
	}
	name = truncateRunes(strings.TrimSpace(name), 80)
	amount = truncateRunes(strings.TrimSpace(amount), 80)
	if name == "" {
		return errors.New("请填写要买的食材")
	}
	var n int64
	if err := database.DB.Model(&models.FamilyShoppingItem{}).Where("family_id = ?", family.ID).Count(&n).Error; err != nil {
		return err
	}
	if n >= 300 {
		return errors.New("买菜清单已达 300 项上限")
	}
	item := models.FamilyShoppingItem{FamilyID: family.ID, Name: name, Amount: amount, AddedBy: uid}
	return database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "family_id"}, {Name: "name"}},
		DoUpdates: clause.Assignments(map[string]any{"amount": amount, "checked": false, "added_by": uid}),
	}).Create(&item).Error
}

func SetFamilyShoppingChecked(uid, itemID uint, checked bool) error {
	family, err := RequireFamily(uid)
	if err != nil {
		return err
	}
	res := database.DB.Model(&models.FamilyShoppingItem{}).Where("id = ? AND family_id = ?", itemID, family.ID).Update("checked", checked)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("清单项不存在")
	}
	return nil
}

func DeleteFamilyShopping(uid, itemID uint) error {
	family, err := RequireFamily(uid)
	if err != nil {
		return err
	}
	res := database.DB.Where("id = ? AND family_id = ?", itemID, family.ID).Delete(&models.FamilyShoppingItem{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("清单项不存在")
	}
	return nil
}

func ImportFamilyIngredients(uid, dishID uint) (int, error) {
	family, err := RequireFamily(uid)
	if err != nil {
		return 0, err
	}
	var dish models.Dish
	if err := database.DB.Where("id = ? AND ((owner_id = 0 AND family_id = 0) OR family_id = ?)", dishID, family.ID).First(&dish).Error; err != nil {
		return 0, ErrFamilyDish
	}
	count := 0
	for _, raw := range []string{dish.Ingredients, dish.Seasonings} {
		var objects []nameAmount
		if err := json.Unmarshal([]byte(raw), &objects); err != nil {
			for _, name := range ingredientNames(raw) {
				objects = append(objects, nameAmount{Name: name})
			}
		}
		for _, item := range objects {
			if strings.TrimSpace(item.Name) != "" {
				if err := AddFamilyShopping(uid, item.Name, item.Amount); err != nil {
					return count, err
				}
				count++
			}
		}
	}
	return count, nil
}
