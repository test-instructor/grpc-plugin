package main

import (
	"encoding/json"
	"fmt"
	"github.com/test-instructor/grpc-plugin/demo"
	"github.com/test-instructor/grpc-plugin/plugin"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

func main() {
	go demo.StartSvc()
	defer demo.StopSvc()
	rand.Seed(time.Now().UnixNano())
	var g = &plugin.Grpc{}
	req := make(map[string]interface{})
	req["UserName"] = strconv.Itoa(rand.Intn(1000000))
	req["Password"] = "1112"
	reqStr, err := json.Marshal(req)
	g.Host = "127.0.0.1:40061"
	g.Method = "user.User.RegisterUser"
	g.Timeout = 1.0
	g.Metadata = []plugin.RpcMetadata{{"User", "test"}}
	g.Body = strings.NewReader(string(reqStr))
	ig := plugin.NewInvokeGrpc(g)
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
	req["Password"] = "111112"
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
