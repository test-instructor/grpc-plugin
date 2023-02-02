package plugin

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/desc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	insecurecreds "google.golang.org/grpc/credentials/insecure"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	// Register gzip compressor so compressed responses will work
	_ "google.golang.org/grpc/encoding/gzip"
	// Register xds so xds and xds-experimental resolver schemes work
	_ "google.golang.org/grpc/xds"

	"github.com/fullstorydev/grpcui/standalone"
)

var version = "dev build <no version set>"

var exit = os.Exit
var services, methods []string

type multiString []string

func (s *multiString) String() string {
	return strings.Join(*s, ",")
}

func (s *multiString) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type optionalBoolFlag struct {
	set, val bool
}

func (f *optionalBoolFlag) String() string {
	if !f.set {
		return "unset"
	}
	return strconv.FormatBool(f.val)
}

func (f *optionalBoolFlag) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	f.set = true
	f.val = v
	return nil
}

func (f *optionalBoolFlag) IsBoolFlag() bool {
	return true
}

// Uses a file source as a fallback for resolving symbols and extensions, but
// only uses the reflection source for listing services
type compositeSource struct {
	reflection grpcurl.DescriptorSource
	file       grpcurl.DescriptorSource
}

func (cs compositeSource) ListServices() ([]string, error) {
	return cs.reflection.ListServices()
}

func (cs compositeSource) FindSymbol(fullyQualifiedName string) (desc.Descriptor, error) {
	d, err := cs.reflection.FindSymbol(fullyQualifiedName)
	if err == nil {
		return d, nil
	}
	return cs.file.FindSymbol(fullyQualifiedName)
}

func (cs compositeSource) AllExtensionsForType(typeName string) ([]*desc.FieldDescriptor, error) {
	exts, err := cs.reflection.AllExtensionsForType(typeName)
	if err != nil {
		// On error fall back to file source
		return cs.file.AllExtensionsForType(typeName)
	}
	// Track the tag numbers from the reflection source
	tags := make(map[int32]bool)
	for _, ext := range exts {
		tags[ext.GetNumber()] = true
	}
	fileExts, err := cs.file.AllExtensionsForType(typeName)
	if err != nil {
		return exts, nil
	}
	for _, ext := range fileExts {
		// Prioritize extensions found via reflection
		if !tags[ext.GetNumber()] {
			exts = append(exts, ext)
		}
	}
	return exts, nil
}

func prettify(docString string) string {
	parts := strings.Split(docString, "\n")

	// cull empty lines and also remove trailing and leading spaces
	// from each line in the doc string
	j := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts[j] = part
		j++
	}

	return strings.Join(parts[:j], "\n")
}

func warn(msg string, args ...interface{}) {
	msg = fmt.Sprintf("Warning: %s\n", msg)
	fmt.Fprintf(os.Stderr, msg, args...)
}

func fail(err error, msg string, args ...interface{}) {
	if err != nil {
		msg += ": %v"
		args = append(args, err)
	}
	fmt.Fprintf(os.Stderr, msg, args...)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		exit(1)
	} else {
		// nil error means it was CLI usage issue
		fmt.Fprintf(os.Stderr, "Try '%s -help' for more details.\n", os.Args[0])
		exit(2)
	}
}

func checkAssetNames(soFar map[string]string, names []string, requireFile bool) {
	for _, n := range names {
		st, err := os.Stat(n)
		if err != nil {
			if os.IsNotExist(err) {
				fail(nil, "File %q does not exist", n)
			} else {
				fail(err, "Failed to check existence of file %q", n)
			}
		}
		if requireFile && st.IsDir() {
			fail(nil, "Path %q is a folder, not a file", n)
		}

		base := filepath.Base(n)
		if existing, ok := soFar[base]; ok {
			fail(nil, "Multiple assets with the same base name specified: %s and %s", existing, n)
		}
		soFar[base] = n
	}
}

func configureJSandCSS(names []string, fn func(string, func() (io.ReadCloser, error)) standalone.HandlerOption) []standalone.HandlerOption {
	opts := make([]standalone.HandlerOption, len(names))
	for i := range names {
		name := names[i] // no loop variable so that we don't close over loop var in lambda below
		open := func() (io.ReadCloser, error) {
			return os.Open(name)
		}
		opts[i] = fn(filepath.Base(name), open)
	}
	return opts
}

func configureAssets(names []string) []standalone.HandlerOption {
	opts := make([]standalone.HandlerOption, len(names))
	for i := range names {
		name := names[i] // no loop variable so that we don't close over loop var in lambdas below
		st, err := os.Stat(name)
		if err != nil {
			fail(err, "failed to inspect file %q", name)
		}
		if st.IsDir() {
			open := func(p string) (io.ReadCloser, error) {
				path := filepath.Join(name, p)
				st, err := os.Stat(path)
				if err == nil && st.IsDir() {
					// Strangely, os.Open does not return an error if given a directory
					// and instead returns an empty reader :(
					// So check that first and return a 404 if user indicates directory name
					return nil, os.ErrNotExist
				}
				return os.Open(path)
			}
			opts[i] = standalone.ServeAssetDirectory(filepath.Base(name), open)
		} else {
			open := func() (io.ReadCloser, error) {
				return os.Open(name)
			}
			opts[i] = standalone.ServeAssetFile(filepath.Base(name), open)
		}
	}
	return opts
}

type svcConfig struct {
	includeService bool
	includeMethods map[string]struct{}
}

func getMethods(source grpcurl.DescriptorSource, configs map[string]*svcConfig) ([]*desc.MethodDescriptor, error) {
	allServices, err := source.ListServices()
	if err != nil {
		return nil, err
	}

	var descs []*desc.MethodDescriptor
	for _, svc := range allServices {
		if svc == "grpc.reflection.v1alpha.ServerReflection" {
			continue
		}
		d, err := source.FindSymbol(svc)
		if err != nil {
			return nil, err
		}
		sd, ok := d.(*desc.ServiceDescriptor)
		if !ok {
			return nil, fmt.Errorf("%s should be a service descriptor but instead is a %T", d.GetFullyQualifiedName(), d)
		}
		cfg := configs[svc]
		if cfg == nil && len(configs) != 0 {
			// not configured to expose this service
			continue
		}
		delete(configs, svc)
		for _, md := range sd.GetMethods() {
			if cfg == nil {
				descs = append(descs, md)
				continue
			}
			_, found := cfg.includeMethods[md.GetName()]
			delete(cfg.includeMethods, md.GetName())
			if found && cfg.includeService {
				warn("Service %s already configured, so -method %s is unnecessary", svc, md.GetName())
			}
			if found || cfg.includeService {
				descs = append(descs, md)
			}
		}
		if cfg != nil && len(cfg.includeMethods) > 0 {
			// configured methods not found
			methodNames := make([]string, 0, len(cfg.includeMethods))
			for m := range cfg.includeMethods {
				methodNames = append(methodNames, fmt.Sprintf("%s/%s", svc, m))
			}
			sort.Strings(methodNames)
			return nil, fmt.Errorf("configured methods not found: %s", strings.Join(methodNames, ", "))
		}
	}

	//if len(configs) > 0 {
	//	// configured services not found
	//	svcNames := make([]string, 0, len(configs))
	//	for s := range configs {
	//		svcNames = append(svcNames, s)
	//	}
	//	sort.Strings(svcNames)
	//	return nil, fmt.Errorf("configured services not found: %s", strings.Join(svcNames, ", "))
	//}

	return descs, nil
}

func computeSvcConfigs() (map[string]*svcConfig, error) {
	if len(services) == 0 && len(methods) == 0 {
		return nil, nil
	}
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

func splitMethodName(name string) (svc, method string) {
	dot := strings.LastIndex(name, ".")
	slash := strings.LastIndex(name, "/")
	sep := dot
	if slash > dot {
		sep = slash
	}
	if sep < 0 {
		return "", name
	}
	return name[:sep], name[sep+1:]
}

type codeSniffer struct {
	w       http.ResponseWriter
	code    int
	codeSet bool
	size    int
}

func (cs *codeSniffer) Header() http.Header {
	return cs.w.Header()
}

func (cs *codeSniffer) Write(b []byte) (int, error) {
	if !cs.codeSet {
		cs.code = 200
		cs.codeSet = true
	}
	cs.size += len(b)
	return cs.w.Write(b)
}

func (cs *codeSniffer) WriteHeader(statusCode int) {
	if !cs.codeSet {
		cs.code = statusCode
		cs.codeSet = true
	}
	cs.w.WriteHeader(statusCode)
}

func dial(ctx context.Context, network, addr string, creds credentials.TransportCredentials, failFast bool, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if failFast {
		return grpcurl.BlockingDial(ctx, network, addr, creds, opts...)
	}
	// BlockingDial will return the first error returned. It is meant for interactive use.
	// If we don't want to fail fast, then we need to do a more customized dial.

	// TODO: perhaps this logic should be added to the grpcurl package, like in a new
	// BlockingDialNoFailFast function?

	dialer := &errTrackingDialer{
		dialer:  &net.Dialer{},
		network: network,
	}
	var errCreds errTrackingCreds
	if creds == nil {
		opts = append(opts, grpc.WithTransportCredentials(insecurecreds.NewCredentials()))
	} else {
		errCreds = errTrackingCreds{
			TransportCredentials: creds,
		}
		opts = append(opts, grpc.WithTransportCredentials(&errCreds))
	}

	cc, err := grpc.DialContext(ctx, addr, append(opts, grpc.WithBlock(), grpc.WithContextDialer(dialer.dial))...)
	if err == nil {
		return cc, nil
	}

	// prefer last observed TLS handshake error if there is one
	if err := errCreds.err(); err != nil {
		return nil, err
	}
	// otherwise, use the error the dialer last observed
	if err := dialer.err(); err != nil {
		return nil, err
	}
	// if we have no better source of error message, use what grpc.DialContext returned
	return nil, err
}

type errTrackingCreds struct {
	credentials.TransportCredentials

	mu      sync.Mutex
	lastErr error
}

func (c *errTrackingCreds) ClientHandshake(ctx context.Context, addr string, rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	conn, auth, err := c.TransportCredentials.ClientHandshake(ctx, addr, rawConn)
	if err != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.lastErr = err
	}
	return conn, auth, err
}

func (c *errTrackingCreds) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

type errTrackingDialer struct {
	dialer  *net.Dialer
	network string

	mu      sync.Mutex
	lastErr error
}

func (c *errTrackingDialer) dial(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := c.dialer.DialContext(ctx, c.network, addr)
	if err != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.lastErr = err
	}
	return conn, err
}

func (c *errTrackingDialer) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

func dumpResponse(r *http.Response, includeBody bool) (string, error) {
	// NB: not using httputil.DumpResponse because it writes binary data in the body which is
	//  not useful in the log (and can cause unexpected behavior when writing to a terminal,
	//  which may interpret some byte sequences as control codes).
	var buf bytes.Buffer
	buf.WriteString(r.Status)
	buf.WriteRune('\n')
	if err := r.Header.Write(&buf); err != nil {
		return "", err
	}
	if includeBody {
		buf.WriteRune('\n')
		ct := strings.ToLower(r.Header.Get("content-type"))
		mt, _, err := mime.ParseMediaType(ct)
		if err != nil {
			mt = ct
		}
		isText := strings.HasPrefix(mt, "text/") ||
			mt == "application/json" ||
			mt == "application/javascript" ||
			mt == "application/x-www-form-urlencoded" ||
			mt == "multipart/form-data" ||
			mt == "application/xml"
		if isText {
			if _, err := io.Copy(&buf, r.Body); err != nil {
				return "", err
			}
		} else {
			first := true
			var block [32]byte
			for {
				n, err := r.Body.Read(block[:])
				if n > 0 {
					if first {
						buf.WriteString("(binary body; encoded in hex)\n")
						first = false
					}
					for i := 0; i < n; i += 8 {
						end := i + 8
						if end > n {
							end = n
						}
						buf.WriteString(hex.EncodeToString(block[i:end]))
						buf.WriteRune(' ')
					}
					buf.WriteRune('\n')
				}
				if err == io.EOF {
					break
				} else if err != nil {
					return "", err
				}
			}
		}
	}
	return buf.String(), nil
}
