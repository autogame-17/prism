package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func TestDefaultProtocolAnthropicRegistersRootMessagesRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	viper.Reset()
	viper.Set("protocol.default", "anthropic")
	t.Cleanup(viper.Reset)

	engine := gin.New()
	SetRelayRouter(engine)

	if !hasRoute(engine, "POST", "/v1/messages") {
		t.Fatalf("expected /v1/messages to be registered when protocol.default=anthropic")
	}
}

func TestDefaultProtocolOpenAIDoesNotRegisterRootMessagesRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	viper.Reset()
	viper.Set("protocol.default", "openai")
	t.Cleanup(viper.Reset)

	engine := gin.New()
	SetRelayRouter(engine)

	if hasRoute(engine, "POST", "/v1/messages") {
		t.Fatalf("expected /v1/messages to remain unregistered by default")
	}
}

func hasRoute(engine *gin.Engine, method string, path string) bool {
	for _, route := range engine.Routes() {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}
