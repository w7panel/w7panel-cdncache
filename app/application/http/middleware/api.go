package middleware

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

var internalApiMap = make(map[string]gin.HandlerFunc)

func RegisterInternalApiRoutes(path string, handler gin.HandlerFunc) {
	internalApiMap[path] = handler
}

type Api struct {
	middleware.Abstract
}

func (c Api) checkToken(ctx *gin.Context) error {
	remoteToken := strings.ToLower(ctx.Request.Header.Get("Authorization"))
	localToken := strings.ToLower(facade.GetConfig().GetString("setting.oauth_token"))

	slog.Info("token info", "remoteToken", remoteToken, "localToken", localToken)
	if remoteToken == localToken {
		return nil
	}

	header := strings.Split(remoteToken, "bearer ")
	if len(header) <= 1 {
		return errors.New("请先登录")
	}
	token := header[1]

	if token == "" || token != localToken {
		return errors.New("非法请求")
	}

	return nil
}

func (c Api) Process(ctx *gin.Context) {
	handler, exist := internalApiMap[ctx.Request.URL.Path]
	if !exist {
		for path, _handler := range internalApiMap {
			path = strings.TrimRight(path, "*")
			if strings.HasPrefix(ctx.Request.URL.Path, path) {
				ctx.Request.URL.Path = strings.TrimPrefix(ctx.Request.URL.Path, path)
				handler = _handler
			}
		}
	}
	if handler != nil {
		err := c.checkToken(ctx)
		if err != nil {
			c.JsonResponseWithServerError(ctx, err)
			ctx.Abort()
			return
		}

		ctx.Set("api_handler", true)
		handler(ctx)
		ctx.Abort()
		return
	}

	if ctx.Request.URL.Path == "/health" {
		c.JsonSuccessResponse(ctx)
		ctx.Abort()
		return
	}

	ctx.Next()
}
