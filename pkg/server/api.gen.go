// Package api provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/oapi-codegen/oapi-codegen/v2 version v2.4.1 DO NOT EDIT.
package api

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gorilla/mux"
	"github.com/oapi-codegen/runtime"
)

// Defines values for ChangeBatteryStatePayloadState.
const (
	Charge          ChangeBatteryStatePayloadState = "charge"
	Discharge       ChangeBatteryStatePayloadState = "discharge"
	SelfConsumption ChangeBatteryStatePayloadState = "self_consumption"
)

// Defines values for ChangeInverterStatePayloadState.
const (
	Off ChangeInverterStatePayloadState = "off"
	On  ChangeInverterStatePayloadState = "on"
)

// ChangeBatteryStatePayload defines model for ChangeBatteryStatePayload.
type ChangeBatteryStatePayload struct {
	Power *string                        `json:"power,omitempty"`
	State ChangeBatteryStatePayloadState `json:"state"`
}

// ChangeBatteryStatePayloadState defines model for ChangeBatteryStatePayload.State.
type ChangeBatteryStatePayloadState string

// ChangeFeedinPayload defines model for ChangeFeedinPayload.
type ChangeFeedinPayload struct {
	Disable bool `json:"disable"`
}

// ChangeInverterStatePayload defines model for ChangeInverterStatePayload.
type ChangeInverterStatePayload struct {
	State ChangeInverterStatePayloadState `json:"state"`
}

// ChangeInverterStatePayloadState defines model for ChangeInverterStatePayload.State.
type ChangeInverterStatePayloadState string

// Empty defines model for Empty.
type Empty = map[string]interface{}

// PostBatteryStateJSONRequestBody defines body for PostBatteryState for application/json ContentType.
type PostBatteryStateJSONRequestBody = ChangeBatteryStatePayload

// PostInverterFeedinJSONRequestBody defines body for PostInverterFeedin for application/json ContentType.
type PostInverterFeedinJSONRequestBody = ChangeFeedinPayload

// PostInverterStateJSONRequestBody defines body for PostInverterState for application/json ContentType.
type PostInverterStateJSONRequestBody = ChangeInverterStatePayload

// ServerInterface represents all server handlers.
type ServerInterface interface {

	// (POST /battery/{state})
	PostBatteryState(w http.ResponseWriter, r *http.Request, state string)

	// (POST /inverter/feedin)
	PostInverterFeedin(w http.ResponseWriter, r *http.Request)

	// (POST /inverter/{state})
	PostInverterState(w http.ResponseWriter, r *http.Request, state string)
}

// ServerInterfaceWrapper converts contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler            ServerInterface
	HandlerMiddlewares []MiddlewareFunc
	ErrorHandlerFunc   func(w http.ResponseWriter, r *http.Request, err error)
}

type MiddlewareFunc func(http.Handler) http.Handler

// PostBatteryState operation middleware
func (siw *ServerInterfaceWrapper) PostBatteryState(w http.ResponseWriter, r *http.Request) {

	var err error

	// ------------- Path parameter "state" -------------
	var state string

	err = runtime.BindStyledParameterWithOptions("simple", "state", mux.Vars(r)["state"], &state, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "state", Err: err})
		return
	}

	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.PostBatteryState(w, r, state)
	}))

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r)
}

// PostInverterFeedin operation middleware
func (siw *ServerInterfaceWrapper) PostInverterFeedin(w http.ResponseWriter, r *http.Request) {

	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.PostInverterFeedin(w, r)
	}))

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r)
}

// PostInverterState operation middleware
func (siw *ServerInterfaceWrapper) PostInverterState(w http.ResponseWriter, r *http.Request) {

	var err error

	// ------------- Path parameter "state" -------------
	var state string

	err = runtime.BindStyledParameterWithOptions("simple", "state", mux.Vars(r)["state"], &state, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "state", Err: err})
		return
	}

	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.PostInverterState(w, r, state)
	}))

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r)
}

type UnescapedCookieParamError struct {
	ParamName string
	Err       error
}

func (e *UnescapedCookieParamError) Error() string {
	return fmt.Sprintf("error unescaping cookie parameter '%s'", e.ParamName)
}

func (e *UnescapedCookieParamError) Unwrap() error {
	return e.Err
}

type UnmarshalingParamError struct {
	ParamName string
	Err       error
}

func (e *UnmarshalingParamError) Error() string {
	return fmt.Sprintf("Error unmarshaling parameter %s as JSON: %s", e.ParamName, e.Err.Error())
}

func (e *UnmarshalingParamError) Unwrap() error {
	return e.Err
}

type RequiredParamError struct {
	ParamName string
}

func (e *RequiredParamError) Error() string {
	return fmt.Sprintf("Query argument %s is required, but not found", e.ParamName)
}

type RequiredHeaderError struct {
	ParamName string
	Err       error
}

func (e *RequiredHeaderError) Error() string {
	return fmt.Sprintf("Header parameter %s is required, but not found", e.ParamName)
}

func (e *RequiredHeaderError) Unwrap() error {
	return e.Err
}

type InvalidParamFormatError struct {
	ParamName string
	Err       error
}

func (e *InvalidParamFormatError) Error() string {
	return fmt.Sprintf("Invalid format for parameter %s: %s", e.ParamName, e.Err.Error())
}

func (e *InvalidParamFormatError) Unwrap() error {
	return e.Err
}

type TooManyValuesForParamError struct {
	ParamName string
	Count     int
}

func (e *TooManyValuesForParamError) Error() string {
	return fmt.Sprintf("Expected one value for %s, got %d", e.ParamName, e.Count)
}

// Handler creates http.Handler with routing matching OpenAPI spec.
func Handler(si ServerInterface) http.Handler {
	return HandlerWithOptions(si, GorillaServerOptions{})
}

type GorillaServerOptions struct {
	BaseURL          string
	BaseRouter       *mux.Router
	Middlewares      []MiddlewareFunc
	ErrorHandlerFunc func(w http.ResponseWriter, r *http.Request, err error)
}

// HandlerFromMux creates http.Handler with routing matching OpenAPI spec based on the provided mux.
func HandlerFromMux(si ServerInterface, r *mux.Router) http.Handler {
	return HandlerWithOptions(si, GorillaServerOptions{
		BaseRouter: r,
	})
}

func HandlerFromMuxWithBaseURL(si ServerInterface, r *mux.Router, baseURL string) http.Handler {
	return HandlerWithOptions(si, GorillaServerOptions{
		BaseURL:    baseURL,
		BaseRouter: r,
	})
}

// HandlerWithOptions creates http.Handler with additional options
func HandlerWithOptions(si ServerInterface, options GorillaServerOptions) http.Handler {
	r := options.BaseRouter

	if r == nil {
		r = mux.NewRouter()
	}
	if options.ErrorHandlerFunc == nil {
		options.ErrorHandlerFunc = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
	wrapper := ServerInterfaceWrapper{
		Handler:            si,
		HandlerMiddlewares: options.Middlewares,
		ErrorHandlerFunc:   options.ErrorHandlerFunc,
	}

	r.HandleFunc(options.BaseURL+"/battery/{state}", wrapper.PostBatteryState).Methods("POST")

	r.HandleFunc(options.BaseURL+"/inverter/feedin", wrapper.PostInverterFeedin).Methods("POST")

	r.HandleFunc(options.BaseURL+"/inverter/{state}", wrapper.PostInverterState).Methods("POST")

	return r
}

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"H4sIAAAAAAAC/9RUzY7bPAx8FYPfd3TjtAX2oOMWLbC3BXLoYREUik0nWtiUKjJpg8DvXojKn5tk0cMW",
	"6J4iRDQ5M5zRDmrfB09IwmB2wPUKe6vHTytLS7y3Ihi3M7GCj3bbedukyxB9wCgOtTT4HxjTAX/aPnQI",
	"5m5yV4JsA4IBluhoCUMJnLpoHa17ME/A2LXfak+87oM4T1BCvbJxiVBC43h/npenxtc+cQQGgpUVXMwc",
	"Soj4fe0iNjpPAcyPZX7xjLUkaJntF8TG0U2ejWO76HDEVOIaj/0W3ndo6WLu4cPbkx9og1Ewviz0hYDK",
	"37ftWKNXU+VzH2Sb5v12kzo4ar1eOdGhXx2hvJsVM4wbjFDCBiOnBRl4P5lOpqmfD0g2ODDwUf8qFaBS",
	"qxbZadVO4QzZVyzpN0lg07IfGjDw6FnObaldou1RMDKYp92IOtk+U8+VJ955cdnwZxSPGs1zMbLc+0ZF",
	"qD0JkiKyIXSuVkzVMyeSu7NW/0dswcB/1Slc1T5Z1e1YqaoNch1ddrbZe6PQsuJYl4Bx8MTZFB+m01dD",
	"lxd+BUmdkaiMxWF8KhxKqNzevFWr+Xl5dQen56zB31R5HOfb+qa6wlHBKOJoyf+yxH8Uj9Fr8hbzcfU5",
	"fKMBGYZfAQAA//8BsunCZgcAAA==",
}

// GetSwagger returns the content of the embedded swagger specification file
// or error if failed to decode
func decodeSpec() ([]byte, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %w", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %w", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %w", err)
	}

	return buf.Bytes(), nil
}

var rawSpec = decodeSpecCached()

// a naive cached of a decoded swagger spec
func decodeSpecCached() func() ([]byte, error) {
	data, err := decodeSpec()
	return func() ([]byte, error) {
		return data, err
	}
}

// Constructs a synthetic filesystem for resolving external references when loading openapi specifications.
func PathToRawSpec(pathToFile string) map[string]func() ([]byte, error) {
	res := make(map[string]func() ([]byte, error))
	if len(pathToFile) > 0 {
		res[pathToFile] = rawSpec
	}

	return res
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file. The external references of Swagger specification are resolved.
// The logic of resolving external references is tightly connected to "import-mapping" feature.
// Externally referenced files must be embedded in the corresponding golang packages.
// Urls can be supported but this task was out of the scope.
func GetSwagger() (swagger *openapi3.T, err error) {
	resolvePath := PathToRawSpec("")

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, url *url.URL) ([]byte, error) {
		pathToFile := url.String()
		pathToFile = path.Clean(pathToFile)
		getSpec, ok := resolvePath[pathToFile]
		if !ok {
			err1 := fmt.Errorf("path not found: %s", pathToFile)
			return nil, err1
		}
		return getSpec()
	}
	var specData []byte
	specData, err = rawSpec()
	if err != nil {
		return
	}
	swagger, err = loader.LoadFromData(specData)
	if err != nil {
		return
	}
	return
}
