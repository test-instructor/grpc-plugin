package plugin

import (
	"context"
	"errors"
	"fmt"
	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"io"
	"net/http"
	"sync"
	"time"
)

var (
	resourceMap     = make(map[string]grpcurl.DescriptorSource)
	ccMap           = make(map[string]*grpc.ClientConn)
	ctxMap          = make(map[string]context.Context)
	resourceRWMutex sync.RWMutex
)

type Grpc struct {
	Host     string
	Method   string
	Metadata []RpcMetadata
	Timeout  float32
	Body     io.Reader
}

type InvokeGrpc struct {
	G          *Grpc
	descSource grpcurl.DescriptorSource
	cc         *grpc.ClientConn
	ctx        context.Context
}

func NewInvokeGrpc(g *Grpc) *InvokeGrpc {
	return &InvokeGrpc{G: g}
}

func (i *InvokeGrpc) getResource() (err error) {
	resourceRWMutex.RLock()
	res := resourceMap[i.G.Host]
	resourceRWMutex.RUnlock()
	if res == nil {
		resourceRWMutex.Lock()
		defer resourceRWMutex.Unlock()
		res = resourceMap[i.G.Host]
		if res == nil {
			err = i.getClient()
			if err != nil {
				return
			}
			resourceMap[i.G.Host] = i.descSource
			ccMap[i.G.Host] = i.cc
			ctxMap[i.G.Host] = i.ctx
		}
	}
	i.descSource = resourceMap[i.G.Host]
	i.cc = ccMap[i.G.Host]
	i.ctx = ctxMap[i.G.Host]
	return
}

func (i *InvokeGrpc) getClient() (err error) {
	dialTime := 10 * time.Second
	keepaliveTime := 0.0
	maxMsgSz := 1024 * 1024 * 256
	ctx := context.Background()
	dialCtx, cancel := context.WithTimeout(ctx, dialTime)
	defer cancel()
	var opts []grpc.DialOption

	if keepaliveTime > 0 {
		timeout := time.Duration(keepaliveTime * float64(time.Second))
		opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    timeout,
			Timeout: timeout,
		}))
	}
	opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSz)))

	network := "tcp"
	var creds credentials.TransportCredentials
	i.cc, err = dial(dialCtx, network, i.G.Host, creds, true, opts...)
	if err != nil {
		return
	}

	var refClient *grpcreflect.Client

	addlHeaders := []string{}
	reflHeaders := []string{}
	md := grpcurl.MetadataFromHeaders(append(addlHeaders, reflHeaders...))
	refCtx := metadata.NewOutgoingContext(ctx, md)
	refClient = grpcreflect.NewClient(refCtx, reflectpb.NewServerReflectionClient(i.cc))
	reflSource := grpcurl.DescriptorSourceFromServer(ctx, refClient)
	i.descSource = compositeSource{reflection: reflSource}
	i.ctx = ctx
	return
}

func (i *InvokeGrpc) InvokeFunction() (results *RpcResult, err error) {
	err = i.getResource()
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	configs, err := ComputeSvcConfigs([]string{i.G.Host}, []string{i.G.Method})
	if err != nil {
		return
	}
	descs, err := getMethods(i.descSource, configs)
	if err != nil {
		return
	}
	for _, md := range descs {
		if md.GetFullyQualifiedName() == i.G.Method {
			i.descSource, err = grpcurl.DescriptorSourceFromFileDescriptors(md.GetFile())

			var input rpcInput
			var js []byte
			js, err = io.ReadAll(i.G.Body)
			if err != nil {
				return
			}
			input.Data = append(input.Data, js)
			input.Metadata = i.G.Metadata
			input.TimeoutSeconds = i.G.Timeout

			results, err = invokeRPC(i.ctx, i.G.Method, i.cc, i.descSource, header, input, &InvokeOptions{})

			return
		}
	}
	return nil, errors.New("未找到对应的请求方式")
}

func ComputeSvcConfigs(services, methods []string) (map[string]*svcConfig, error) {

	configs := map[string]*svcConfig{}
	for _, svc := range services {
		configs[svc] = &svcConfig{
			includeService: true,
			includeMethods: map[string]struct{}{},
		}
	}
	for _, fqMethod := range methods {
		svc, method := splitMethodName(fqMethod)
		if svc == "" || method == "" {
			return nil, fmt.Errorf("could not parse name into service and method names: %q", fqMethod)
		}
		cfg := configs[svc]
		if cfg == nil {
			cfg = &svcConfig{includeMethods: map[string]struct{}{}}
			configs[svc] = cfg
		}
		cfg.includeMethods[method] = struct{}{}
	}
	return configs, nil
}

func (i *InvokeGrpc) getSvs() (svc []string, err error) {
	allServices, err := i.descSource.ListServices()
	if err != nil {
		return
	}
	for _, v := range allServices {
		if v == "grpc.reflection.v1alpha.ServerReflection" {
			continue
		}
		svc = append(svc, v)
	}
	return
}

func (i *InvokeGrpc) getMethod(serverName string) (method []string, err error) {

	d, err := i.descSource.FindSymbol(serverName)
	if err != nil {
		return nil, err
	}
	sd, ok := d.(*desc.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s should be a service descriptor but instead is a %T", d.GetFullyQualifiedName(), d)
	}

	for _, md := range sd.GetMethods() {
		method = append(method, md.GetName())
	}

	return
}

func (i *InvokeGrpc) getReq(svc, method string) (results *schema, err error) {
	d, err := i.descSource.FindSymbol(svc)
	if err != nil {
		return nil, err
	}
	sd, ok := d.(*desc.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s should be a service descriptor but instead is a %T", d.GetFullyQualifiedName(), d)
	}
	for _, md := range sd.GetMethods() {

		if md.GetName() == method {
			r, err := gatherMetadataForMethod(md)
			if err != nil {
				return nil, err
			}

			results = r
			break
		}
	}
	if results == nil {
		return nil, errors.New("results 为空")
	}

	return
}
