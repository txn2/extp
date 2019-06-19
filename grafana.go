package extp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/txn2/ack"
	"go.uber.org/zap"
)

// GraCreateResult
type GraCreateResult struct {
	Org  GraCreateOrgResponse `json:"org"`
	User GraUser              `json:"user"`
}

// GraUserOrgRole
type GraUserOrgRole struct {
	LoginOrEmail string `json:"loginOrEmail"`
	Role         string `json:"role"`
}

// GraUser
type GraUser struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Login    string `json:"login"`
	Password string `json:"password"`
}

// GraOrgAddress
type GraOrgAddress struct {
	Address1 string `json:"address1"`
	Address2 string `json:"address2"`
	City     string `json:"city"`
	ZipCode  string `json:"zipCode"`
	State    string `json:"state"`
	Country  string `json:"country"`
}

// GraOrg
type GraOrg struct {
	Id      int           `json:"id,omitempty"`
	Name    string        `json:"name"`
	Address GraOrgAddress `json:"address,omitempty"`
}

// GraCreateUserResponse
type GraCreateUserResponse struct {
	Id      int    `json:"id"`
	Message string `json:"message"`
}

// GraCreateOrgResponse
type GraCreateOrgResponse struct {
	Message string `json:"message"`
	OrgId   int    `json:"orgId"`
}

// CrafanaClientCfg
type GrafanaClientCfg struct {
	Location string
	Username string
	Password string
	Api      *Api
}

// GrafanaClient
type GrafanaClient struct {
	*GrafanaClientCfg
}

// NewGrafanaClient
func NewGrafanaClient(cfg *GrafanaClientCfg) *GrafanaClient {
	return &GrafanaClient{GrafanaClientCfg: cfg}
}

// EnablePluginHandler
func (gc *GrafanaClient) EnablePluginHandler(c *gin.Context) {
	ak := ack.Gin(c)
	orgName := c.Param("orgName")
	plugin := c.Param("plugin")

	// get the orgId from the org name
	code, resp, err := gc.Cmd("GET", "/api/orgs/name/"+orgName, 0, nil)
	if err != nil {
		ak.GinErrorAbort(500, "GrafanaClientError", err.Error())
		return
	}

	if code != 200 {
		ak.GinErrorAbort(code, "GetOrgNon200", string(*resp))
		return
	}

	gcOrg := &GraOrg{}

	err = json.Unmarshal(*resp, gcOrg)
	if err != nil {
		ak.SetPayload(string(*resp))
		ak.GinErrorAbort(code, "UnmarshalError", err.Error())
	}

	rd, err := c.GetRawData()
	if err != nil {
		ak.SetPayloadType("ErrorMessage")
		ak.SetPayload("There was a problem with the posted data")
		ak.GinErrorAbort(500, "PostDataError", err.Error())
		return
	}

	// Enable a plugin
	code, resp, err = gc.Cmd("POST", "/api/plugins/"+plugin+"/settings", gcOrg.Id, rd)
	if err != nil {
		ak.GinErrorAbort(500, "GrafanaClientError", err.Error())
		return
	}

	if code != 200 {
		ak.GinErrorAbort(code, "EnablePluginNon200", string(*resp))
		return
	}

	ak.SetPayload(resp)
}

// CreateOrgHandler creates a Grafana organization and default user.
// @TODO rollbacks
func (gc *GrafanaClient) CreateOrgHandler(c *gin.Context) {
	ak := ack.Gin(c)

	orgName := c.Param("orgName")

	org := GraOrg{
		Name: orgName,
	}

	// Create an organization
	// https://grafana.com/docs/http_api/org/#create-organization
	code, resp, err := gc.CmdObj("POST", "/api/orgs", 0, org)
	if err != nil {
		ak.GinErrorAbort(500, "GrafanaClientError", err.Error())
		return
	}

	if code != 200 {
		ak.GinErrorAbort(code, "Non200", string(*resp))
		return
	}

	gcOrg := &GraCreateOrgResponse{}

	err = json.Unmarshal(*resp, gcOrg)
	if err != nil {
		ak.SetPayload(string(*resp))
		ak.GinErrorAbort(code, "UnmarshalError", err.Error())
	}

	user := GraUser{
		Name:     orgName,
		Login:    orgName,
		Password: "12345",
	}

	// Create a global user
	// https://grafana.com/docs/http_api/admin/#global-users
	code, resp, err = gc.CmdObj("POST", "/api/admin/users", 0, user)
	if err != nil {
		ak.GinErrorAbort(500, "GrafanaClientError", err.Error())
		return
	}

	if code != 200 {
		// @TODO rollback?
		ak.GinErrorAbort(code, "Non200", string(*resp))
		return
	}

	gcUsr := &GraCreateUserResponse{}

	err = json.Unmarshal(*resp, gcUsr)
	if err != nil {
		// @TODO rollback?
		ak.SetPayload(string(*resp))
		ak.GinErrorAbort(code, "Non200", err.Error())
	}

	gcLoginRole := &GraUserOrgRole{
		LoginOrEmail: orgName,
		Role:         "Viewer",
	}

	// Remove new global user from org 1
	// https://grafana.com/docs/http_api/org/#delete-user-in-organization
	code, resp, err = gc.CmdObj("DELETE",
		fmt.Sprintf("/api/orgs/1/users/%d", gcUsr.Id),
		0,
		gcLoginRole,
	)
	if err != nil {
		// @TODO rollback?
		ak.GinErrorAbort(500, "GrafanaClientError", err.Error())
		return
	}

	if code != 200 {
		// @TODO rollback?
		ak.GinErrorAbort(code, "Non200", string(*resp))
		return
	}

	// Update the new org with the new user.
	// https://grafana.com/docs/http_api/org/#update-users-in-organization
	code, resp, err = gc.CmdObj("POST",
		fmt.Sprintf("/api/orgs/%d/users", gcOrg.OrgId),
		0,
		gcLoginRole,
	)
	if err != nil {
		// @TODO rollback?
		ak.GinErrorAbort(500, "GrafanaClientError", err.Error())
		return
	}

	if code != 200 {
		// @TODO rollback?
		ak.GinErrorAbort(code, "Non200", string(*resp))
		return
	}

	gcr := GraCreateResult{
		Org:  *gcOrg,
		User: user,
	}

	ak.SetPayloadType("GraCreateResult")
	ak.GinSend(gcr)
}

func (gc *GrafanaClient) CmdObj(verb string, path string, orgId int, payload interface{}) (int, *[]byte, error) {
	payloadJs, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}

	return gc.Cmd(verb, path, orgId, payloadJs)
}

// GrafanaClient
func (gc *GrafanaClient) Cmd(verb string, path string, orgId int, payloadJs []byte) (int, *[]byte, error) {

	gc.Api.Logger.Info("Grafana Communication",
		zap.String("verb", verb),
		zap.String("path", path),
		zap.ByteString("json", payloadJs),
	)

	req, err := http.NewRequest(verb, "http://localhost:3000"+path, bytes.NewBuffer(payloadJs))
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	if orgId > 0 {
		gc.Api.Logger.Info("Set Organization Header", zap.Int("org_id", orgId))
		req.Header.Set("X-Grafana-Org-Id", strconv.Itoa(orgId))
	}

	req.SetBasicAuth("admin", "admin")

	resp, err := gc.Api.HttpClient.Http.Do(req)
	if err != nil {
		return 0, nil, err
	}

	if resp != nil {
		defer resp.Body.Close()
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, &body, nil
}
