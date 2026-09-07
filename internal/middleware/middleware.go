package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/victorhugomazzeo/goexpert_03/internal/limiter"
)

const MessageLimitReached = "you have reached the maximum number of requests or actions allowed within a certain time frame"

func New(l *limiter.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := limiter.Request{
				IP:    clientIP(r),
				Token: r.Header.Get("API_KEY"),
			}

			decision, err := l.Allow(r.Context(), req)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			if !decision.Allowed {
				w.Header().Set("Retry-After", retryAfterSeconds(decision.RetryAfter))
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(MessageLimitReached))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extrai o IP de r.RemoteAddr, que normalmente vem como "ip:porta".
// Sem porta, usa o valor como esta: middleware de infraestrutura degrada,
// nao recusa requisicao valida por detalhe de formato de endereco.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// retryAfterSeconds converte para o formato do header Retry-After
// (segundos inteiros), arredondando para CIMA: para baixo o cliente
// voltaria antes da hora e levaria 429 de novo.
func retryAfterSeconds(d time.Duration) string {
	s := int(math.Ceil(d.Seconds()))
	if s < 1 {
		s = 1
	}
	return strconv.Itoa(s)
}
