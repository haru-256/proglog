// Package server provides HTTP server implementation for the proglog service.
// It includes in-memory log storage and basic CRUD operations.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// NewHTTPServer creates and configures a new HTTP server with the specified address.
// It sets up RESTful API routes for producing and consuming log records, applies
// logging middleware to all requests, and returns a configured HTTP server.
//
// API Endpoints:
//   - POST /  : Append a new record to the log (handleProduce)
//   - GET  /  : Retrieve a record by offset (handleConsume)
//
// The server includes request logging middleware that captures method, URI,
// duration, and other request details for debugging and monitoring.
func NewHTTPServer(addr string) *http.Server {
	httpsrv := newHTTPServer()
	r := mux.NewRouter()
	// ロギングミドルウェアをルーターに登録
	// このミドルウェアは、この後に定義される全てのルートに適用されます。
	r.Use(loggingMiddleware)
	r.HandleFunc("/", httpsrv.handleProduce).Methods("POST")
	r.HandleFunc("/", httpsrv.handleConsume).Methods("GET")
	return &http.Server{Addr: addr, Handler: r}
}

// httpServer handles HTTP requests for log operations.
// It wraps a Log instance to provide RESTful API endpoints for log management.
// The server uses an in-memory log implementation suitable for development and testing.
type httpServer struct{ Log *Log }

func newHTTPServer() *httpServer {
	return &httpServer{Log: NewLog()}
}

// handleProduce processes POST requests to append new records to the log.
// It expects a JSON-encoded ProduceRequest in the request body containing the record to append.
// On success, it returns a JSON-encoded ProduceResponse with the assigned offset.
//
// Request format:  {"record": {"value": "base64-encoded-data"}}
// Response format: {"offset": 123}
//
// HTTP Status Codes:
//   - 200: Success - record appended successfully
//   - 400: Bad Request - invalid JSON or malformed request
//   - 500: Internal Server Error - log operation failed
func (s *httpServer) handleProduce(w http.ResponseWriter, r *http.Request) {
	defer closeRequestBody(r)
	var req ProduceRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	off, err := s.Log.Append(req.Record)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res := ProduceResponse{Offset: off}
	err = json.NewEncoder(w).Encode(res)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleConsume processes GET requests to retrieve records from the log.
// It expects a JSON-encoded ConsumeRequest in the request body specifying the offset to read.
// On success, it returns a JSON-encoded ConsumeResponse containing the requested record.
//
// Request format:  {"offset": 123}
// Response format: {"record": {"value": "base64-encoded-data", "offset": 123}}
//
// HTTP Status Codes:
//   - 200: Success - record retrieved successfully
//   - 400: Bad Request - invalid JSON or malformed request
//   - 404: Not Found - requested offset does not exist
//   - 500: Internal Server Error - log operation failed
func (s *httpServer) handleConsume(w http.ResponseWriter, r *http.Request) {
	defer closeRequestBody(r)
	var req ConsumeRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := s.Log.Read(req.Offset)
	if err == ErrOffsetNotFound {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res := ConsumeResponse{Record: record}
	err = json.NewEncoder(w).Encode(res)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// ProduceRequest represents the request payload for appending a record to the log.
type ProduceRequest struct {
	Record Record `json:"record"` // The record to be appended
}

// ProduceResponse represents the response payload after successfully appending a record.
type ProduceResponse struct {
	Offset uint64 `json:"offset"` // The assigned offset of the appended record
}

// ConsumeRequest represents the request payload for retrieving a record from the log.
type ConsumeRequest struct {
	Offset uint64 `json:"offset"` // The offset of the record to retrieve
}

// ConsumeResponse represents the response payload containing the requested record.
type ConsumeResponse struct {
	Record Record `json:"record"` // The retrieved record
}

// closeRequestBody safely closes the request body and logs any errors.
// This function should be called with defer to ensure proper resource cleanup.
func closeRequestBody(r *http.Request) {
	if r.Body != nil {
		if err := r.Body.Close(); err != nil {
			slog.Error("error closing request body", "error", err)
		}
	}
}

// loggingMiddleware is middleware that logs request information including method, URI, and duration.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// 次のハンドラを呼び出す
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		// ログ出力
		slog.Debug("request", "method", r.Method, "uri", r.RequestURI, "body", r.Body, "remote_addr", r.RemoteAddr, "user_agent", r.UserAgent(), "duration", duration)
	})
}
