FROM golang:1.23-alpine

WORKDIR /app
COPY go.mod ./
COPY go.sum ./

RUN go mod download
COPY *.go ./

# You need CGO_ENABLED=0 to make it so the binary isn't dynamically linked
# for more information: https://stackoverflow.com/a/55106860/57626
RUN CGO_ENABLED=0 GOOS=linux go build -o /airgradient2mqtt

FROM scratch

COPY --from=0 /airgradient2mqtt /airgradient2mqtt

CMD ["/airgradient2mqtt", "-config", "/config.toml"]