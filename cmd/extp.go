package main

import (
	"flag"
	"os"

	"github.com/txn2/extp"
	"github.com/txn2/micro"
)

var (
	elasticServerEnv = getEnv("ELASTIC_SERVER", "http://elasticsearch:9200")
	systemPrefixEnv  = getEnv("SYSTEM_PREFIX", "system_")

	graLocationEnv      = getEnv("GF_LOCATION", "http://localhost")
	graAdminUserEnv     = getEnv("GF_SECURITY_ADMIN_USER", "admin")
	graAdminPasswordEnv = getEnv("GF_SECURITY_ADMIN_PASSWORD", "admin")
)

func main() {
	esServer := flag.String("esServer", elasticServerEnv, "Elasticsearch Server")
	systemPrefix := flag.String("systemPrefix", systemPrefixEnv, "Prefix for system indices.")
	graLocation := flag.String("graLocation", graLocationEnv, "Grafana location.")
	graAdminUser := flag.String("graAdminUser", graAdminUserEnv, "Grafana admin user.")
	graAdminPassword := flag.String("graAdminPassword", graAdminPasswordEnv, "Grafana location")

	serverCfg, _ := micro.NewServerCfg("External Component Provisioning")
	server := micro.NewServer(serverCfg)

	// External Component Provisioning API
	extpApi, err := extp.NewApi(&extp.Config{
		Logger:        server.Logger,
		HttpClient:    server.Client,
		ElasticServer: *esServer,
		IdxPrefix:     *systemPrefix,
		Token:         server.Token,
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

	// @TODO lookup orgName as an account, find the parent and check the API (should be a key of the parent)

	server.Router.GET("/grafana/createOrg/:orgName", graClient.CreateOrgHandler)

	server.Router.POST("/grafana/enablePlugin/:orgName/:plugin", graClient.EnablePluginHandler)

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
