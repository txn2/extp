package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/txn2/ack"
	"github.com/txn2/extp"
	"github.com/txn2/micro"
	"github.com/txn2/provision"
	"go.uber.org/zap"
)

var (
	graLocationEnv      = getEnv("GF_LOCATION", "http://localhost")
	graAdminUserEnv     = getEnv("GF_SECURITY_ADMIN_USER", "admin")
	graAdminPasswordEnv = getEnv("GF_SECURITY_ADMIN_PASSWORD", "admin")

	// extp uses the txn2/provision service to authenticate Account keys used to
	// authenticate the requester. See: https://github.com/txn2/provision
	provisionServiceEnv = getEnv("PROVISION_SERVICE", "http://api-provision:8070")
)

func main() {
	graLocation := flag.String("graLocation", graLocationEnv, "Grafana location.")
	graAdminUser := flag.String("graAdminUser", graAdminUserEnv, "Grafana admin user.")
	graAdminPassword := flag.String("graAdminPassword", graAdminPasswordEnv, "Grafana location")
	provisionService := flag.String("provisionService", provisionServiceEnv, "Provision service.")

	serverCfg, _ := micro.NewServerCfg("External Component Provisioning")
	server := micro.NewServer(serverCfg)

	// External Component Provisioning API
	extpApi, err := extp.NewApi(&extp.Config{
		Logger:     server.Logger,
		HttpClient: server.Client,
	})
	if err != nil {
		server.Logger.Fatal("failure to instantiate the provisioning API: " + err.Error())
		os.Exit(1)
	}

	server.Router.GET("/welcome", extpApi.WelcomeHandler)

	graClient := extp.NewGrafanaClient(&extp.GrafanaClientCfg{
		Location: *graLocation,
		Username: *graAdminUser,
		Password: *graAdminPassword,
		Api:      extpApi,
	})

	// in-memory cache for authentication
	csh := cache.New(1*time.Minute, 10*time.Minute)

	// get the parent id from the account and and check the
	// access key against it.
	checkAccount := func(childAccountId string, accessKey provision.AccessKey) (bool, error) {

		// the cache key represents permission for this accessKey to
		// manage the accountId
		cacheKey := childAccountId + accessKey.Name + accessKey.Key

		// check cache
		cacheResult, found := csh.Get(cacheKey)
		if found {
			return cacheResult.(bool), nil
		}

		// get the parent for childAccountId
		getAccountUrl := *provisionService + "/account/" + childAccountId
		server.Logger.Info("Looking up child account", zap.String("account_lookup", getAccountUrl))

		req, _ := http.NewRequest("GET", getAccountUrl, nil)
		res, err := server.Client.Http.Do(req)
		if err != nil {
			server.Logger.Warn(
				"Provision service request failure.",
				zap.String("url", getAccountUrl),
				zap.Error(err))

			csh.Set(cacheKey, false, cache.DefaultExpiration)
			return false, err
		}

		if res.StatusCode != 200 {
			return false, errors.New("failed to lookup account: " + getAccountUrl)
		}

		childAccountResult := &provision.AccountResultAck{}

		if res != nil {
			defer res.Body.Close()
		}

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return false, err
		}

		err = json.Unmarshal(body, childAccountResult)
		if err != nil {
			return false, err
		}

		// got the child account, now check the key against it's parentId
		parentAccountId := childAccountResult.Payload.Source.Parent

		if parentAccountId == "" {
			return false, errors.New("account does not have a parent")
		}

		url := *provisionService + "/keyCheck/" + parentAccountId

		accountKeyJson, err := json.Marshal(accessKey)
		if err != nil {
			csh.Set(cacheKey, false, cache.DefaultExpiration)
			return false, err
		}

		req, _ = http.NewRequest("POST", url, bytes.NewReader(accountKeyJson))
		res, err = server.Client.Http.Do(req)
		if err != nil {
			server.Logger.Warn(
				"Provision service request failure.",
				zap.String("url", url),
				zap.Error(err))

			csh.Set(cacheKey, false, cache.DefaultExpiration)
			return false, err
		}

		if res.StatusCode == 404 {
			csh.Set(cacheKey, false, cache.DefaultExpiration)
			return false, errors.New(parentAccountId + " account not found.")
		}

		if res.StatusCode == 200 {
			csh.Set(cacheKey, true, cache.DefaultExpiration)
			return true, nil
		}

		csh.Set(cacheKey, false, cache.DefaultExpiration)
		return false, fmt.Errorf("got code %d from %s ", res.StatusCode, url)
	}

	accessHandler := func(c *gin.Context) {
		ak := ack.Gin(c)

		accountId := c.Param("orgName")
		server.Logger.Info("Org / Account check", zap.String("account", accountId))

		// Check api key for parentAccount if one exists
		name, key, ok := c.Request.BasicAuth()
		if ok {
			accessKey := provision.AccessKey{
				Name: name,
				Key:  key,
			}

			ok, err := checkAccount(accountId, accessKey)
			if err != nil {
				ak.SetPayloadType("ErrorMessage")
				ak.SetPayload("APIKeyCheckError")
				ak.GinErrorAbort(401, "E401", err.Error())
				return
			}

			if !ok {
				ak.SetPayloadType("ErrorMessage")
				ak.SetPayload("APIKeyCheckError")
				ak.GinErrorAbort(401, "E401", "Invalid API Key")
				return
			}

			// valid key
			return
		}
	}

	server.Router.GET(
		"/grafana/createOrg/:orgName",
		// verifies access to orgName
		accessHandler,
		graClient.CreateOrgHandler,
	)

	server.Router.POST(
		"/grafana/enablePlugin/:orgName/:plugin",
		// verifies access to orgName
		accessHandler,
		graClient.EnablePluginHandler,
	)

	server.Router.POST(
		"/grafana/createDatasource/:orgName",
		// verifies access to orgName
		accessHandler,
		graClient.CreateDatasourceHandler,
	)

	server.Router.GET(
		"/grafana/setHomeDashboard/:orgName/:uid",
		// verifies access to orgName
		accessHandler,
		graClient.HomeDashboardHandler,
	)

	// run provisioning server
	server.Run()
}

// getEnv gets an environment variable or sets a default if
// one does not exist.
func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}

	return value
}
