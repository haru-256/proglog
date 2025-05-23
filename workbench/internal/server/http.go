package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

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

type httpServer struct{ Log *Log }

func newHTTPServer() *httpServer {
	return &httpServer{Log: NewLog()}
}

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

type ProduceRequest struct {
	Record Record `json:"record"`
}
type ProduceResponse struct {
	Offset uint64 `json:"offset"`
}
type ConsumeRequest struct {
	Offset uint64 `json:"offset"`
}
type ConsumeResponse struct {
	Record Record `json:"record"`
}

func closeRequestBody(r *http.Request) {
	if r.Body != nil {
		if err := r.Body.Close(); err != nil {
			slog.Error("error closing request body", "error", err)
		}
	}
}

// loggingMiddleware はリクエストの情報をログに出力するミドルウェアです。
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
