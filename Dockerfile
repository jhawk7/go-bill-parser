FROM golang:alpine AS builder
WORKDIR /go/app
COPY . ./
RUN go mod download
RUN mkdir bin
RUN cd cmd/bill-parser/ && go build -o ../../bin/bill-parser

FROM golang:alpine
WORKDIR /go
COPY --from=builder /go/app/bin/bill-parser .
CMD ["./bill-parser"]