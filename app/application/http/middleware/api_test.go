package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPublicApiRouteDoesNotRequireAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := "/api/test/public-without-authorization"
	RegisterPublicApiRoutes(path, func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"public": true})
	})

	engine := gin.New()
	engine.Any("/*path", Api{}.Process)
	request := httptest.NewRequest(http.MethodPost, path, nil)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("public API returned status %d, want %d", response.Code, http.StatusOK)
	}
}
