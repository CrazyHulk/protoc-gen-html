# protoc-gen-html

## install

```bash
go get github.com/lvht/protoc-gen-markdown
```

## generate html

```bash
protoc --html_out=. hello.proto
# set path prefix to /api
protoc --html_out=path_prefix=/api:. hello.proto
```

## how to test 
go build . && protoc --plugin=./protoc-gen-markdown --markdown_out=. activity.proto && mv activity.md activity.html
