package app

import (
	"gitee.com/we7coreteam/w7-cdn-cache/app/application/http/controller"
	"gitee.com/we7coreteam/w7-cdn-cache/app/application/http/middleware"
	"gitee.com/we7coreteam/w7-cdn-cache/app/application/logic"
	"github.com/gin-gonic/gin"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/response"
	httpServer "github.com/we7coreteam/w7-rangine-go/v2/src/http/server"
)

type Provider struct {
}

func (p Provider) Register(httpServer *httpServer.Server) {
	response.SetErrResponseHandler(func(ctx *gin.Context, env string, err error, statusCode int) {
		if ctx.GetBool("api_handler") {
			ctx.JSON(statusCode, map[string]interface{}{
				"error": err.Error(),
				"code":  statusCode,
			})
		} else {
			type Response struct {
				Error string `xml:"error"`
				Code  any    `xml:"code"`
			}
			ctx.XML(statusCode, Response{
				Error: err.Error(),
				Code:  statusCode,
			})
		}
	})

	p.RegisterHttpRoutes(httpServer)

	logic.Transfer{}.Loop()
}

func (p Provider) RegisterHttpRoutes(server *httpServer.Server) {
	middleware.RegisterInternalApiRoutes("/api/setting/set", controller.Setting{}.Set)
	middleware.RegisterInternalApiRoutes("/api/setting/get", controller.Setting{}.Get)
	middleware.RegisterInternalApiRoutes("/api/setting/list", controller.Setting{}.List)
	middleware.RegisterInternalApiRoutes("/api/setting/del", controller.Setting{}.Del)
	middleware.RegisterInternalApiRoutes("/api/util/clear-file-cache", controller.File{}.ClearFileCache)
	middleware.RegisterInternalApiRoutes("/api/k8s/proxy/*", controller.K8s{}.Proxy)

	server.RegisterRouters(func(engine *gin.Engine) {
		engine.Any("/*path", middleware.Cors{}.Process, middleware.Api{}.Process, controller.File{}.Download)
	})
}
