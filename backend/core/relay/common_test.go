package relay

import (
	"net/http"
	"testing"

	"one-api/types"

	"github.com/gin-gonic/gin"
)

func TestFilterOpenAIErrPreservesPaymentRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	filtered := FilterOpenAIErr(c, &types.OpenAIErrorWithStatusCode{
		OpenAIError: types.OpenAIError{
			Message: "user quota is not enough",
			Type:    "one_hub_error",
			Code:    "insufficient_user_quota",
		},
		StatusCode: http.StatusPaymentRequired,
	})

	if filtered.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("expected 402 to be preserved, got %d", filtered.StatusCode)
	}
	if filtered.Code != "insufficient_user_quota" {
		t.Fatalf("expected original error code to be preserved, got %v", filtered.Code)
	}
}
