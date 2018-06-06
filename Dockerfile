FROM golang:alpine

RUN apk --no-cache add git

ADD main.go /go/src/github.com/kaihendry/lk2/main.go
ADD public /go/src/github.com/kaihendry/lk2/public
WORKDIR /go/src/github.com/kaihendry/lk2
RUN go get github.com/rakyll/statik
RUN go generate
RUN go get -d -v
RUN go install -v

FROM alpine:latest
COPY --from=0 /go/bin/lk2 /go/bin/lk2

ARG COMMIT
ENV COMMIT ${COMMIT}

ENV PORT 9000
CMD ["/go/bin/lk2"]
