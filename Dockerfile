FROM golang
COPY . /go/src/github.com/nalind/metacopy-check
RUN go build -o /metacopy-check -ldflags "-linkmode external -extldflags -static" /go/src/github.com/nalind/metacopy-check
FROM busybox
COPY --from=0 /metacopy-check /
CMD /metacopy-check /
