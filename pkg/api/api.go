package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/fatedier/frp/pkg/msg"
)

type Service struct {
	Host url.URL
}

func NewService(host string) (s *Service, err error) {
	u, err := url.Parse(host)
	if err != nil {
		return
	}
	return &Service{*u}, nil
}

// CheckToken 校验客户端 token
func (s Service) CheckToken(user string, token string, timestamp int64, stk string) (ok bool, err error) {
	values := url.Values{}
	values.Set("action", "checktoken")
	values.Set("user", user)
	values.Set("token", token)
	values.Set("timestamp", fmt.Sprintf("%d", timestamp))
	values.Set("apitoken", stk)
	s.Host.RawQuery = values.Encode()
	defer func(u *url.URL) {
		u.RawQuery = ""
	}(&s.Host)
	resp, err := http.Get(s.Host.String())
	err := Body.Close()
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, ErrHTTPStatus{
			Status: resp.StatusCode,
			Text:   resp.Status,
		}
	}
	defer func(Body io.ReadCloser) {
		if err != nil {
			fmt.Println(err)
		}
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	response := ResponseCheckToken{}
	if err = json.Unmarshal(body, &response); err != nil {
		return false, err
	}
	if !response.Success {
		return false, ErrCheckTokenFail{response.Message}
	}
	return true, nil
}

// CheckProxy 校验客户端代理
func (s Service) CheckProxy(user string, pMsg *msg.NewProxy, timestamp int64, stk string, loginMsg *msg.Login) (ok bool, err error) {

	domains, err := json.Marshal(pMsg.CustomDomains)
	if err != nil {
		return false, err
	}

	values := url.Values{}

	// API Basic
	values.Set("action", "checkproxy")
	values.Set("user", user)
	values.Set("timestamp", fmt.Sprintf("%d", timestamp))
	values.Set("apitoken", stk)

	// Proxies basic info
	values.Set("proxy_name", pMsg.ProxyName)
	values.Set("proxy_type", pMsg.ProxyType)

	// Proxies login info
	values.Set("run_id", loginMsg.RunID)

	// Load balance
	values.Set("group", pMsg.Group)
	values.Set("group_key", pMsg.GroupKey)

	switch pMsg.ProxyType {
	case "http", "https":
		// Http代理
		values.Set("domain", string(domains))
	case "tcp", "udp":
		// Tcp和Udp
		values.Set("remote_port", strconv.Itoa(pMsg.RemotePort))
	case "stcp", "xtcp":
		// Stcp和Xtcp
		values.Set("remote_port", strconv.Itoa(pMsg.RemotePort))
		values.Set("sk", pMsg.Sk)
	}
	s.Host.RawQuery = values.Encode()
	defer func(u *url.URL) {
		u.RawQuery = ""
	}(&s.Host)
	resp, err := http.Get(s.Host.String())
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, ErrHTTPStatus{
			Status: resp.StatusCode,
			Text:   resp.Status,
		}
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	response := ResponseCheckProxy{}
	if err = json.Unmarshal(body, &response); err != nil {
		return false, err
	}
	if !response.Success {
		return false, ErrCheckProxyFail{response.Message}
	}
	return true, nil
}

// GetProxyLimit 获取隧道限速信息
func (s Service) GetProxyLimit(user string, timestamp int64, stk string) (inLimit, outLimit uint64, err error) {
	values := url.Values{}
	values.Set("action", "getlimit")
	values.Set("user", user)
	values.Set("timestamp", fmt.Sprintf("%d", timestamp))
	values.Set("apitoken", stk)
	s.Host.RawQuery = values.Encode()
	defer func(u *url.URL) {
		u.RawQuery = ""
	}(&s.Host)
	resp, err := http.Get(s.Host.String())
	if err != nil {
		return 0, 0, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}

	er := &ErrHTTPStatus{}
	if err = json.Unmarshal(body, er); err != nil {
		return 0, 0, err
	}
	if er.Status != 200 {
		return 0, 0, er
	}

	response := &ResponseGetLimit{}
	if err = json.Unmarshal(body, response); err != nil {
		return 0, 0, err
	}

	// 这里直接返回 uint64 应该问题不大
	return response.MaxIn, response.MaxOut, nil
}

func BoolToString(val bool) (str string) {
	if val {
		return "true"
	}
	return "false"

}

type ErrHTTPStatus struct {
	Status int    `json:"status"`
	Text   string `json:"message"`
}

func (e ErrHTTPStatus) Error() string {
	return fmt.Sprintf("ME Frp API Error (Status: %d, Text: %s)", e.Status, e.Text)
}

type ResponseGetLimit struct {
	MaxIn  uint64 `json:"max-in"`
	MaxOut uint64 `json:"max-out"`
}

type ResponseCheckToken struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ResponseCheckProxy struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ErrCheckTokenFail struct {
	Message string
}

type ErrCheckProxyFail struct {
	Message string
}

func (e ErrCheckTokenFail) Error() string {
	return e.Message
}

func (e ErrCheckProxyFail) Error() string {
	return e.Message
}
