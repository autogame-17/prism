package fakectx

import (
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

// New constructs a *gin.Context backed by an httptest.ResponseRecorder, suitable
// for calling one-hub controllers in-process (e.g. channel test).
func New() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}
