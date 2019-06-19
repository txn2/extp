package extp

import (
	"github.com/gin-gonic/gin"
	"github.com/txn2/ack"
	"github.com/txn2/es/v2"
	"github.com/txn2/micro"
	"github.com/txn2/token"
	"go.uber.org/zap"
)

// Config
type Config struct {
	Logger     *zap.Logger
	HttpClient *micro.Client

	// used for communication with Elasticsearch
	// if nil, one will be created
	Elastic       *es.Client
	ElasticServer string

	// used to prefix the user and account indexes IdxPrefix_user, IdxPrefix_account
	// defaults to system.
	IdxPrefix string

	// pre-configured from server (txn2/micro)
	Token *token.Jwt
}

// Api
type Api struct {
	*Config
}

// NewApi
func NewApi(cfg *Config) (*Api, error) {
	a := &Api{Config: cfg}

	if a.Elastic == nil {
		// Configure an elastic client
		a.Elastic = es.CreateClient(es.Config{
			Log:           cfg.Logger,
			HttpClient:    cfg.HttpClient.Http,
			ElasticServer: cfg.ElasticServer,
		})
	}

	if cfg.IdxPrefix == "" {
		cfg.IdxPrefix = "system_"
	}

	return a, nil
}

// PrefixHandler
func (a *Api) WelcomeHandler(c *gin.Context) {
	ak := ack.Gin(c)
	ak.SetPayloadType("Message")
	ak.GinSend("Welcome")
}
