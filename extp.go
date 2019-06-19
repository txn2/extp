package extp

import (
	"github.com/gin-gonic/gin"
	"github.com/txn2/ack"
	"github.com/txn2/micro"
	"go.uber.org/zap"
)

// Config
type Config struct {
	Logger     *zap.Logger
	HttpClient *micro.Client
}

// Api
type Api struct {
	*Config
}

// NewApi
func NewApi(cfg *Config) (*Api, error) {
	a := &Api{Config: cfg}

	return a, nil
}

// PrefixHandler
func (a *Api) WelcomeHandler(c *gin.Context) {
	ak := ack.Gin(c)
	ak.SetPayloadType("Message")
	ak.GinSend("Welcome")
}
