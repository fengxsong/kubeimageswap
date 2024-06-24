# build
FROM golang:1.22 as builder
ARG LDFLAGS

WORKDIR /workspace
COPY go.mod go.sum /workspace/
RUN go mod download
COPY main.go mutatingwebhook.go /workspace/
RUN CGO_ENABLED=0 go build -a -ldflags "${LDFLAGS}" -o kubeimageswap && ./kubeimageswap --version

# run
FROM alpine:3

COPY --from=builder /workspace/kubeimageswap /kubeimageswap

LABEL author="fengxsong <fengxsong@outlook.com>"

EXPOSE 9443
ENTRYPOINT [ "/kubeimageswap" ]