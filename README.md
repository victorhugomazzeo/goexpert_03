# Rate Limiter

Rate limiter em Go, aplicável como middleware HTTP, que limita requisições por
**endereço IP** ou por **token de acesso**. A contagem pode ser guardada em
memória ou no Redis, e trocar entre as duas é uma variável de ambiente — sem
recompilar e sem tocar no código.

## Como funciona

Toda requisição passa pelo middleware antes de chegar ao roteador. O limitador
resolve **uma** identidade para a requisição e conta em cima dela:

- se o header `API_KEY` traz um token **configurado**, o limite é o daquele token
- caso contrário, o limite é o do IP de origem

O token sempre ganha do IP: um token com limite de 100 req/s passa mesmo que o
IP dele já tenha estourado. Token desconhecido é ignorado e a requisição cai no
limite de IP.

Estourado o limite, a resposta é:

```
HTTP/1.1 429 Too Many Requests
Content-Type: text/plain; charset=utf-8
Retry-After: 30

you have reached the maximum number of requests or actions allowed within a certain time frame
```

O `Retry-After` vem em segundos, arredondado para cima — para baixo o cliente
voltaria antes da hora e levaria outro 429.

### O algoritmo

Duas durações governam cada identidade:

- **janela**: por quanto tempo as requisições são somadas (`RATE_LIMIT_WINDOW`)
- **bloqueio**: quanto tempo a identidade fica barrada depois de estourar

Quem excede o limite não espera apenas o fim da janela: fica bloqueado pela
duração do bloqueio, que costuma ser bem maior. Servido o bloqueio, o contador
recomeça do zero.

### Fluxo

```
requisição
    │
    ▼
middleware  ──►  limiter  ──►  resolve a identidade (token vence IP)
    │                              │
    │                              ▼
    │                          Strategy (interface)
    │                              ├── redis    contagem compartilhada
    │                              └── memory   contagem local ao processo
    │
    ├── 429 + Retry-After         (o roteador nunca é chamado)
    │
    ▼
ServeMux  ──►  handler
```

## Requisitos

- Docker e Docker Compose, **ou**
- Go 1.26 e um Redis acessível

## Subindo com Docker

```bash
docker compose up -d --build
```

A aplicação fica em **http://localhost:8081**. A porta do host é 8081 (e não
8080) porque 8080 é comum estar ocupada; dentro do container a aplicação escuta
na porta de `SERVER_PORT`. Para mudar o lado do host, edite o mapeamento em
[docker-compose.yaml](docker-compose.yaml).

```bash
docker compose logs -f app     # acompanha o log
docker compose restart app     # aplica mudança no .env, que é lido no boot
docker compose down            # derruba tudo
```

## Rodando na máquina

O Redis pode vir do próprio compose:

```bash
docker compose up -d redis
go run ./cmd/server
```

Rode **da raiz do projeto**: o `.env` é procurado no diretório de trabalho do
processo. Rodando de dentro de `cmd/server`, nenhuma variável é encontrada e
tudo cai nos valores padrão — inclusive `REDIS_ADDR=redis:6379`, que só resolve
dentro da rede do Docker.

Para experimentar sem Redis, sobreponha a strategy no terminal em vez de editar
o arquivo:

```bash
RATE_LIMITER_STRATEGY=memory go run ./cmd/server
```

```powershell
$env:RATE_LIMITER_STRATEGY="memory"; go run ./cmd/server
```

Variável de ambiente já definida **vence** o `.env`: o carregamento não
sobrescreve o que o ambiente já traz. É o mesmo mecanismo que o compose usa para
apontar a aplicação para `redis:6379` sem alterar o arquivo.

## Configuração

Tudo vem de variáveis de ambiente, com um [.env](.env) de exemplo no
repositório.

| variável | padrão | o que faz |
|---|---|---|
| `SERVER_PORT` | `8080` | porta HTTP |
| `RATE_LIMITER_STRATEGY` | `redis` | onde a contagem é guardada: `redis` ou `memory` |
| `REDIS_ADDR` | `redis:6379` | endereço do Redis |
| `REDIS_PASSWORD` | vazio | senha, se houver |
| `REDIS_DB` | `0` | banco do Redis |
| `RATE_LIMIT_WINDOW` | `1s` | janela de contagem |
| `RATE_LIMIT_IP_MAX_REQUESTS` | `10` | requisições por janela, por IP |
| `RATE_LIMIT_IP_BLOCK_DURATION` | `5m` | bloqueio do IP que estoura |
| `RATE_LIMIT_TOKENS` | vazio | limites por token (formato abaixo) |

Os tokens vão numa string só, separados por vírgula, cada um no formato
`token:requisições:bloqueio`:

```
RATE_LIMIT_TOKENS=abc123:100:5m,premium456:1000:1m,fraco789:2:30s
```

Lido assim: `abc123` pode 100 requisições por janela e, se estourar, fica 5
minutos de fora. Formato inválido derruba a aplicação no boot, em vez de a
deixar rodando com um limite que ninguém configurou.

## Testando

O caminho mais rápido para ver um 429 é o token de limite baixo:

```bash
for i in $(seq 4); do curl -s -o /dev/null -w '%{http_code} ' \
  -H 'API_KEY: fraco789' http://localhost:8081/; done; echo
```

```powershell
1..4 | ForEach-Object { curl.exe -s -o NUL -w "%{http_code} " `
  -H "API_KEY: fraco789" http://localhost:8081/ }
```

Saída esperada: `200 200 429 429`.

Para o limite de IP são 11 requisições dentro da janela de 1 segundo, e aí mora
uma pegadinha: cada `curl` é um processo novo, e no Windows abrir processo custa
o suficiente para a janela expirar no meio do laço — dando a impressão de que o
limitador não funciona. Ou mande tudo numa chamada só, reaproveitando a conexão:

```bash
curl -s -w '%{http_code} ' $(for i in $(seq 14); do \
  printf -- '-o /dev/null http://localhost:8081/ '; done); echo
```

ou suba o servidor com uma janela folgada só para o teste manual:

```bash
RATE_LIMIT_WINDOW=10s go run ./cmd/server
```

Para acompanhar o Redis enquanto testa:

```bash
docker exec -it redis_container redis-cli MONITOR     # comandos em tempo real
docker exec redis_container redis-cli KEYS 'ratelimit:*'
```

No PowerShell, use `curl.exe` e não `curl` — no PowerShell 5.1 `curl` é apelido
de `Invoke-WebRequest`, que tem outras flags.

## Testes automatizados

```bash
go test ./...
```

As duas strategies são verificadas pela **mesma** suíte de conformidade
([internal/strategy/strategytest](internal/strategy/strategytest)): é o que
prova que são intercambiáveis, e não apenas que cada uma passa nos seus próprios
testes.

O teste do Redis precisa de um Redis de verdade. Sem ele, o teste é ignorado com
uma mensagem, em vez de falhar:

```bash
docker compose up -d redis
go test ./...
```

Endereço configurável por `REDIS_TEST_ADDR` (padrão `127.0.0.1:6379`). O teste
isola suas chaves por prefixo próprio, então não apaga nada do que estiver no
Redis.

## Estrutura

```
cmd/server/              main: carrega config, escolhe a strategy, sobe o HTTP
internal/config/         leitura e validação das variáveis de ambiente
internal/limiter/        o domínio: Limiter, Rules, Limit, State, Strategy
internal/middleware/     o middleware HTTP e o 429
internal/strategy/
    memory/              contagem em memória, com mutex e limpeza periódica
    redis/               contagem no Redis, via script Lua atômico
    strategytest/        suíte de conformidade compartilhada
```

`internal/limiter` define o domínio e a interface `Strategy`, e não depende de
nada externo. As implementações ficam nos pacotes de fora e são escolhidas em
`main` — acrescentar uma terceira forma de guardar a contagem (Memcached, um
banco) é escrever o pacote e um `case` no `switch`, sem tocar no limitador nem
no middleware.

### Por que a strategy Redis usa um script Lua

O Redis executa um script inteiro sem intercalar outros comandos. Isso importa
porque a decisão envolve mais de uma escrita: contar a requisição e, se ela
estourou, trocar o prazo da chave pelo prazo do bloqueio. Em chamadas separadas,
uma requisição concorrente pode escrever seu prazo entre as duas e sobrescrever
um bloqueio recém-armado, derrubando minutos de punição para o resto de uma
janela. Como efeito colateral, o script gasta uma ida ao Redis por requisição em
vez de três.

Cada identidade ocupa **uma** chave, e é o valor dela que diz em que estado ela
está: até o limite é contagem de janela, acima do limite é bloqueio. O prazo de
validade da chave serve aos dois papéis, e é ele que devolve o `Retry-After` —
nenhuma instância da aplicação precisa ler o próprio relógio, e por isso máquinas
com horários diferentes continuam concordando. Quando o prazo vence, o Redis
apaga a chave e a contagem recomeça do zero sem ninguém precisar zerar nada.

## Limitações conhecidas

**O IP é o da conexão TCP.** A identidade vem de `RemoteAddr`, que atrás de
qualquer proxy reverso — nginx, load balancer, ou o próprio redirecionamento de
portas do Docker — é o endereço do proxy, não o do cliente. Com a aplicação em
container, todo tráfego que chega do host aparece com o IP do gateway da rede
Docker e cai num balde só.

Ler `X-Forwarded-For` resolveria, mas confiar nesse header sem critério é pior
que o problema: qualquer cliente burla o limitador trocando de identidade a cada
requisição com um `-H "X-Forwarded-For: ..."`. Uma implementação correta só
confia nesse header quando a conexão vem de um proxy conhecido.

**Janela ou limite zerados desativam a contagem em silêncio.** Com
`RATE_LIMIT_WINDOW=0`, cada requisição abre uma janela nova e nada é limitado —
nas duas strategies, sem aviso nenhum no log.

**A memória não é compartilhada.** Com `RATE_LIMITER_STRATEGY=memory`, cada
processo conta sozinho: com N réplicas atrás de um balanceador, o limite real
passa a ser N vezes o configurado, e reiniciar a aplicação perdoa quem estava
bloqueado. É exatamente o problema que a strategy Redis existe para resolver.
