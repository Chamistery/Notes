# notes

Сервис заметок с интерфейсами HTTP/0.9, HTTP/1.1, HTTP/2, HTTP/3 и gRPC.

## Требования перед установкой

Обязательно только Go 1.26 или новее и `make`:

```sh
brew install go            # macOS; на Linux — пакет golang или https://go.dev/dl
go version                 # должно быть go1.26 или выше
```

Зависимости Go (grpc, quic-go, x/net) скачиваются автоматически при первой сборке, нужен доступ в интернет.

Для примеров из этого README дополнительно используются `curl` и `nc`, они есть в macOS из коробки.
`protoc`, `grpcurl` и curl с HTTP/3 ставить не нужно: сгенерированный gRPC-код уже в репозитории, а для HTTP/3 и gRPC есть свои клиенты в `bin/`.

## Сборка и запуск

```sh
cd ~/nets
make build build-clients   # соберёт bin/notes, bin/h3client, bin/grpcclient
make test                  # прогонит тесты всех протоколов
make run                   # запустит сервис (или ./bin/notes)
```

Примеры ниже выполняются во втором терминале, пока сервис запущен.

Порты по умолчанию (меняются флагами `-http09`, `-http11`, `-http2`, `-http3`, `-grpc`):

| Протокол | Адрес |
|----------|-------|
| HTTP/0.9 | tcp `:8009` |
| HTTP/1.1 | tcp `:8011` |
| HTTP/2 (h2c) | tcp `:8020` |
| HTTP/3 | udp `:8030` |
| gRPC | tcp `:9090` |

Данные хранятся в памяти. Остановка по `Ctrl+C`.

## REST API (HTTP/0.9, 1.1, 2, 3)

| Метод | Путь | Ответ |
|-------|------|-------|
| `GET` | `/notes` | список |
| `POST` | `/notes` | 201, заголовки `ETag`, `Location` |
| `GET` | `/notes/{id}` | 200 + `ETag`; с `If-None-Match` → 304 |
| `PUT` / `PATCH` | `/notes/{id}` | 200 + новый `ETag`; `If-Match` не совпал → 412 |
| `DELETE` | `/notes/{id}` | 204; `If-Match` не совпал → 412 |

Тело: `{"title":"...","content":"..."}` или `<note><title>..</title><content>..</content></note>`.
Формат ответа выбирается заголовком `Accept: application/json` или `Accept: application/xml`.

### HTTP/0.9

```sh
printf 'GET /notes\r\n' | nc 127.0.0.1 8009
curl --http0.9 http://127.0.0.1:8009/notes
```

### HTTP/1.1

```sh
curl -i --http1.1 -X POST http://127.0.0.1:8011/notes \
  -H 'Content-Type: application/json' -d '{"title":"first","content":"hello"}'

curl --http1.1 http://127.0.0.1:8011/notes -H 'Accept: application/xml'

curl -i --http1.1 http://127.0.0.1:8011/notes/1 -H 'If-None-Match: "<etag>"'

curl -i --http1.1 -X PUT http://127.0.0.1:8011/notes/1 \
  -H 'Content-Type: application/json' -H 'If-Match: "<etag>"' \
  -d '{"title":"second","content":"updated"}'

curl -i --http1.1 -X DELETE http://127.0.0.1:8011/notes/1 -H 'If-Match: "<etag>"'
```

### HTTP/2

```sh
curl -i --http2-prior-knowledge http://127.0.0.1:8020/notes
curl -i --http2-prior-knowledge -X POST http://127.0.0.1:8020/notes \
  -H 'Content-Type: application/json' -d '{"title":"h2","content":"frames"}'
curl -i --http2-prior-knowledge -X DELETE http://127.0.0.1:8020/notes/1 -H 'If-Match: *'
```

### HTTP/3

Системный curl на macOS без HTTP/3, поэтому есть свой клиент с теми же флагами:

```sh
./bin/h3client https://127.0.0.1:8030/notes
./bin/h3client -X POST -d '{"title":"h3","content":"udp"}' https://127.0.0.1:8030/notes
./bin/h3client -X PUT -H 'If-Match: "<etag>"' -d '{"title":"h3","content":"v2"}' https://127.0.0.1:8030/notes/1
./bin/h3client -X DELETE -H 'If-Match: *' https://127.0.0.1:8030/notes/1
```

С curl из Homebrew: `curl -k --http3-only https://127.0.0.1:8030/notes`.

## gRPC

Схема в `api/proto/notes.proto`, reflection включён.

```sh
./bin/grpcclient create "title" "content"
./bin/grpcclient list
./bin/grpcclient get 1
./bin/grpcclient update 1 '"<etag>"' "new title" "new content"
./bin/grpcclient delete 1 '*'
```

Или через grpcurl:

```sh
grpcurl -plaintext -d '{"title":"g","content":"rpc"}' 127.0.0.1:9090 notes.v1.NotesService/CreateNote
grpcurl -plaintext 127.0.0.1:9090 notes.v1.NotesService/ListNotes
grpcurl -plaintext -d '{"id":"1","if_match":"*"}' 127.0.0.1:9090 notes.v1.NotesService/DeleteNote
```

Поле `if_match` в `UpdateNote` и `DeleteNote` работает как заголовок `If-Match`; несовпадение даёт `FAILED_PRECONDITION`.

## Перегенерация protobuf

Нужна только после правки `notes.proto`. Требует `protoc` (`brew install protobuf`):

```sh
make tools    # ставит protoc-gen-go и protoc-gen-go-grpc в $(go env GOPATH)/bin
make proto
```
