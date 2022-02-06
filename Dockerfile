FROM golang:1.17

WORKDIR /go/src/github.com/rdbell/expensive-relay
COPY . .

RUN go install -v

CMD ["expensive-relay"]
