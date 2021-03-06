package lux_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/sirupsen/logrus"

	"github.com/davidsbond/lux"
	"github.com/stretchr/testify/assert"
)

func TestRouter_UsesMiddleware(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Middleware     lux.HandlerFunc
		Request        lux.Request
		Handlers       map[string]lux.HandlerFunc
		ExpectedBody   string
		ExpectedStatus int
	}{
		// Scenario 1: Valid request & happy path middleware
		{
			Request: lux.Request{
				APIGatewayProxyRequest: events.APIGatewayProxyRequest{
					HTTPMethod: "GET",
					Headers:    map[string]string{"content-type": "application/json"},
				},
			},
			Handlers:       map[string]lux.HandlerFunc{"GET": getHandler},
			ExpectedStatus: http.StatusOK,
			Middleware:     middleware,
			ExpectedBody:   "\"hello test\"\n",
		},
		// Scenario 2: Valid request but middleware returns an error.
		{
			Request: lux.Request{
				APIGatewayProxyRequest: events.APIGatewayProxyRequest{
					HTTPMethod: "GET",
					Headers:    map[string]string{"content-type": "application/json"},
				},
			},
			Handlers:       map[string]lux.HandlerFunc{"GET": getHandler},
			ExpectedStatus: http.StatusInternalServerError,
			Middleware:     errorMiddleware,
			ExpectedBody:   "\"error\"",
		},
	}

	for _, tc := range tt {
		// GIVEN that we have a router
		router := lux.NewRouter()
		router.Logging(bytes.NewBuffer([]byte{}), &logrus.JSONFormatter{})

		// AND that router has registered handlers
		for method, handler := range tc.Handlers {
			router.Handler(method, handler).
				Headers("content-type", "application/json").
				Middleware(tc.Middleware)
		}

		// AND the router has registered middleware
		router.Middleware(tc.Middleware)

		// WHEN we perform a request
		resp, _ := router.ServeHTTP(tc.Request)

		// THEN the status code & body should be what we expect.
		assert.Equal(t, tc.ExpectedBody, resp.Body)
		assert.Equal(t, tc.ExpectedStatus, resp.StatusCode)
	}
}

func TestRouter_HandlesRequests(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Request        lux.Request
		Handlers       map[string]lux.HandlerFunc
		ExpectedError  string
		ExpectedStatus int
	}{
		// Scenario 1: Valid GET request with correct headers.
		{
			Request: lux.Request{
				APIGatewayProxyRequest: events.APIGatewayProxyRequest{
					HTTPMethod:            "GET",
					Headers:               map[string]string{"content-type": "application/json"},
					QueryStringParameters: map[string]string{"key": "value"},
				},
			},
			Handlers:       map[string]lux.HandlerFunc{"GET": getHandler},
			ExpectedStatus: http.StatusOK,
		},
		// Scenario 2: Invalid GET request with incorrect header value.
		{
			Request: lux.Request{
				APIGatewayProxyRequest: events.APIGatewayProxyRequest{
					HTTPMethod:            "GET",
					Headers:               map[string]string{"content-type": "application/xml"},
					QueryStringParameters: map[string]string{"key": "value"},
				},
			},
			Handlers:       map[string]lux.HandlerFunc{"GET": getHandler},
			ExpectedStatus: http.StatusNotAcceptable,
			ExpectedError:  "not acceptable",
		},
		// Scenario 3: Handler does not exist
		{
			Request: lux.Request{
				APIGatewayProxyRequest: events.APIGatewayProxyRequest{
					HTTPMethod:            "GET",
					Headers:               map[string]string{"content-type": "application/json"},
					QueryStringParameters: map[string]string{"key": "value"},
				},
			},
			Handlers:       map[string]lux.HandlerFunc{},
			ExpectedStatus: http.StatusMethodNotAllowed,
			ExpectedError:  "not allowed",
		},
		// Scenario 4: Invalid GET request with no headers.
		{
			Request: lux.Request{
				APIGatewayProxyRequest: events.APIGatewayProxyRequest{
					HTTPMethod:            "GET",
					Headers:               map[string]string{},
					QueryStringParameters: map[string]string{"key": "value"},
				},
			},
			Handlers:       map[string]lux.HandlerFunc{"GET": getHandler},
			ExpectedStatus: http.StatusNotAcceptable,
			ExpectedError:  "not acceptable",
		},
		// Scenario 5: Valid DELETE request with only a GET handler registered.
		{
			Request: lux.Request{
				APIGatewayProxyRequest: events.APIGatewayProxyRequest{
					HTTPMethod:            "DELETE",
					Headers:               map[string]string{"content-type": "application/json"},
					QueryStringParameters: map[string]string{"key": "value"},
				},
			},
			Handlers:       map[string]lux.HandlerFunc{"GET": getHandler},
			ExpectedStatus: http.StatusMethodNotAllowed,
			ExpectedError:  "not allowed",
		},
		// Scenario 6: Valid GET request with missing required query params.
		{
			Request: lux.Request{
				APIGatewayProxyRequest: events.APIGatewayProxyRequest{
					HTTPMethod:            "GET",
					Headers:               map[string]string{"content-type": "application/json"},
					QueryStringParameters: map[string]string{},
				},
			},
			Handlers:       map[string]lux.HandlerFunc{"GET": getHandler},
			ExpectedStatus: http.StatusNotAcceptable,
			ExpectedError:  "not acceptable",
		},
	}

	for _, tc := range tt {
		// GIVEN that we have a router
		router := lux.NewRouter()
		router.Logging(bytes.NewBuffer([]byte{}), &logrus.JSONFormatter{})

		// AND that router has handlers registered
		for method, handler := range tc.Handlers {
			router.Handler(method, handler).
				Headers("content-type", "application/json").
				Queries("key", "value")
		}

		// WHEN we perform the request
		resp, err := router.ServeHTTP(tc.Request)

		// THEN any errors should be what we expect
		if err != nil {
			assert.Equal(t, tc.ExpectedError, err.Error())
		}

		// AND the status code should be what we expect.
		assert.Equal(t, tc.ExpectedStatus, resp.StatusCode)
	}
}

func TestRouter_Recovers(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Request        lux.Request
		Handlers       map[string]lux.HandlerFunc
		ExpectedError  string
		ExpectedStatus int
	}{
		// Scenario 1: Handler panics
		{
			Request: lux.Request{
				APIGatewayProxyRequest: events.APIGatewayProxyRequest{
					HTTPMethod: "GET",
					Headers:    map[string]string{"content-type": "application/json"},
				},
			},
			Handlers:       map[string]lux.HandlerFunc{"GET": panicHandler},
			ExpectedStatus: http.StatusInternalServerError,
			ExpectedError:  "failed to obtain response",
		},
	}

	for _, tc := range tt {
		// GIVEN that we have a router with a recovery handler.
		router := lux.NewRouter().Recovery(recoverHandler)
		router.Logging(bytes.NewBuffer([]byte{}), &logrus.JSONFormatter{})

		// AND that router has handlers registered
		for method, handler := range tc.Handlers {
			router.Handler(method, handler).Headers("content-type", "application/json")
		}

		// WHEN we perform the request that will panic
		resp, _ := router.ServeHTTP(tc.Request)

		// AND the status code should be what we expect.
		assert.Equal(t, tc.ExpectedStatus, resp.StatusCode)
		assert.Equal(t, tc.ExpectedError, resp.Body)
	}
}

func getHandler(w lux.ResponseWriter, r *lux.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.Encode("hello test")

}

func recoverHandler(info lux.PanicInfo) {
	// Do nothing
}

func panicHandler(w lux.ResponseWriter, r *lux.Request) {
	panic("uh oh")
}

func errorMiddleware(w lux.ResponseWriter, r *lux.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("\"error\""))
}

func middleware(w lux.ResponseWriter, r *lux.Request) {

}
