// Package redis implementa limiter.Strategy sobre o Redis, para que varias
// instancias da aplicacao contem requisicoes no mesmo lugar em vez de cada
// uma limitar contra a propria memoria.
package redis

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/victorhugomazzeo/goexpert_03/internal/limiter"
)

var _ limiter.Strategy = (*Store)(nil)

// DefaultPrefix da nome de familia as chaves desta strategy, para o mesmo
// Redis poder servir outras coisas -- ou outra copia desta app -- sem
// colisao.
const DefaultPrefix = "ratelimit"

// checkScript e o arquivo check.lua embutido no binario. Fica em arquivo
// separado, e nao num const aqui, por dois motivos: o editor destaca a
// sintaxe do Lua, e ele continua executavel direto pelo redis-cli
// (`redis-cli --eval check.lua chave , 3 1000 5000`), o que torna a
// depuracao muito mais rapida do que passar pelo Go.
//
//go:embed check.lua
var checkScript string

// Options descreve a conexao. Prefix e opcional e cai em DefaultPrefix.
type Options struct {
	Addr     string
	Password string
	DB       int
	Prefix   string
}

type Store struct {
	client *goredis.Client
	script *goredis.Script
	prefix string
}

// New conecta e confere a conexao na hora. Um rate limiter que so descobre
// o Redis fora do ar na primeira requisicao responde 500 para um usuario;
// falhando aqui, ele falha no boot, onde alguem esta olhando.
func New(ctx context.Context, opts Options) (*Store, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     opts.Addr,
		Password: opts.Password,
		DB:       opts.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis %s: %w", opts.Addr, err)
	}

	prefix := opts.Prefix
	if prefix == "" {
		prefix = DefaultPrefix
	}

	return &Store{
		client: client,
		script: goredis.NewScript(checkScript),
		prefix: prefix,
	}, nil
}

// Check delega a decisao inteira ao script, que roda atomico dentro do
// Redis. Os prazos viajam em milissegundos porque e a unidade em que o
// Redis conta TTL; duracao menor que 1ms trunca para zero, e zero o Redis
// entende como "apague a chave".
func (s *Store) Check(ctx context.Context, key string, limit limiter.Limit, window time.Duration) (limiter.State, error) {
	// Run manda EVALSHA e so envia o corpo do script quando o Redis ainda
	// nao o tem em cache -- na primeira chamada do processo, ou depois de um
	// restart do servidor.
	res, err := s.script.Run(ctx, s.client,
		[]string{s.key(key)},
		limit.MaxRequests,
		window.Milliseconds(),
		limit.BlockDuration.Milliseconds(),
	).Int64Slice()
	if err != nil {
		return limiter.State{}, fmt.Errorf("rate limit check %q: %w", key, err)
	}
	if len(res) != 4 {
		return limiter.State{}, fmt.Errorf("rate limit check %q: script devolveu %d valores, esperado 4", key, len(res))
	}

	return limiter.State{
		Allowed:    res[0] == 1,
		Blocked:    res[1] == 1,
		Hits:       res[2],
		RetryAfter: time.Duration(res[3]) * time.Millisecond,
	}, nil
}

func (s *Store) Reset(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, s.key(key)).Err(); err != nil {
		return fmt.Errorf("rate limit reset %q: %w", key, err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.client.Close()
}

// key monta a chave real no Redis. Como o script toca uma unica chave, nao
// e preciso hash tag: qualquer que seja o slot dela, esta todo em um no, e
// o script continua valido sob Redis Cluster.
func (s *Store) key(key string) string {
	return s.prefix + ":" + key
}
