package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMealDraftUsesBoundedAIPoolWithoutBlockingPageLoads(t *testing.T) {
	for _, row := range []struct {
		path    string
		seconds int
		ai      bool
	}{{"/api/health/meal-drafts/parse", 40, true}, {"/api/health/meal-drafts/confirm", 15, false}, {"/api/health/meal-drafts/status", 15, false}} {
		r := gin.New()
		r.Use(RequestBudget())
		r.POST(row.path, func(c *gin.Context) {
			deadline, ok := c.Request.Context().Deadline()
			remaining := time.Until(deadline)
			if !ok || remaining > time.Duration(row.seconds)*time.Second || remaining < time.Duration(row.seconds-1)*time.Second {
				t.Fatalf("wrong deadline: %s %v", row.path, remaining)
			}
			if row.ai {
				if len(streamSlots) != 1 || len(apiSlots) != 0 {
					t.Fatal("draft consumed regular page slot")
				}
			} else if len(apiSlots) != 1 || len(streamSlots) != 0 {
				t.Fatal("normal page consumed AI slot")
			}
			c.Status(204)
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", row.path, nil))
		if w.Code != 204 {
			t.Fatalf("status %d", w.Code)
		}
		if len(streamSlots) != 0 || len(apiSlots) != 0 {
			t.Fatal("request slot leaked")
		}
	}
}
