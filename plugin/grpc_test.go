package plugin

import (
	"encoding/json"
	"fmt"
	"github.com/test-instructor/grpc-plugin/demo"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGrpcConnect(t *testing.T) {
	go demo.StartSvc()
	defer demo.StopSvc()
	rand.Seed(time.Now().UnixNano())
	var g = &Grpc{}
	req := make(map[string]interface{})
	req["UserName"] = strconv.Itoa(rand.Intn(1000000))
	req["P"] = "1112"
	reqStr, err := json.Marshal(req)
	g.Host = "127.0.0.1:40061"
	g.Method = "user.User.RegisterUser"
	g.Timeout = 1.0
	g.Metadata = []RpcMetadata{{"User", "test"}}
	g.Body = strings.NewReader(string(reqStr))
	ig := NewInvokeGrpc(g)
	res, err := ig.InvokeFunction()
	fmt.Println("===================")
	if err == nil && res.Responses != nil {
		fmt.Println(res.Headers)
		for _, v := range res.Responses {
			fmt.Println(string(v.Data))
		}
	} else {
		fmt.Println(err)
	}

	fmt.Println("===================")
	g.Method = "user.User.Login"
	req["P"] = "111112"
	reqStr, err = json.Marshal(req)
	ig.G.Body = strings.NewReader(string(reqStr))
	ig.G.Method = "user.User.Login"
	res2, err2 := ig.InvokeFunction()
	fmt.Println("Name", res2.Error.Name)
	fmt.Println("Code", res2.Error.Code)
	fmt.Println("Message", res2.Error.Message)
	fmt.Println("Details", res2.Error.Details)
	if err2 == nil && res2.Responses != nil {
		fmt.Println(res.Headers)
		for _, v := range res2.Responses {
			fmt.Println(string(v.Data))
		}
	} else {

		fmt.Println(err2)
	}
	fmt.Println("===================")
}

func TestServer(t *testing.T) {
	go demo.StartSvc()
	defer demo.StopSvc()
	var g = &Grpc{}
	g.Host = "127.0.0.1:40061"
	g.Timeout = 1.0

	ig := NewInvokeGrpc(g)
	err := ig.GetResource()
	if err != nil {
		return
	}
	fmt.Println(ig)

	// 获取服务列表
	svc, _ := ig.GetSvs()
	var serverName, method string
	fmt.Println(svc)

	//服务列表不为空时取第一个服务
	if svc != nil && len(svc) > 0 {
		serverName = svc[0]
	}
	//获取method列表并取第一个methon
	methods, _ := ig.GetMethod(serverName)

	if methods != nil && len(methods) > 0 {
		method = methods[0]
	}
	//config, _ := ComputeSvcConfig(ig.g.Host, method)
	//获取req内容
	results, _ := ig.GetReq(serverName, method)
	fmt.Println(results.MessageTypes)
	resultsJson, _ := json.Marshal(results)
	fmt.Println(string(resultsJson))
}

func TestServerReset(t *testing.T) {
	//go demo.StartSvc()
	//defer demo.StopSvc()
	var g = &Grpc{}
	g.Host = "127.0.0.1:40061"
	g.Timeout = 1.0

	ig := NewInvokeGrpc(g)
	err := ig.GetResource()
	if err != nil {
		return
	}
	fmt.Println(ig)

	// 获取服务列表
	svc, _ := ig.GetSvs()
	var serverName, method string
	fmt.Println(svc)

	//服务列表不为空时取第一个服务
	if svc != nil && len(svc) > 0 {
		serverName = svc[0]
	}
	method = "RegisterUser"

	//config, _ := ComputeSvcConfig(ig.g.Host, method)
	//获取req内容
	results, _ := ig.GetReq(serverName, method)
	fmt.Println(results.MessageTypes)
	resultsJson, _ := json.Marshal(results)
	fmt.Println(string(resultsJson))

	err = ig.Reset()
	if err != nil {
		return
	}
	results2, _ := ig.GetReq(serverName, method)
	fmt.Println(results2.MessageTypes)
	resultsJson2, _ := json.Marshal(results2)
	fmt.Println(string(resultsJson2))

}
