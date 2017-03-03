FROM golang:1.7-alpine

COPY . /go/src/github.com/yarpc/yab

RUN apk update
RUN apk add git

RUN cd /go/src/github.com/yarpc/yab && go get github.com/Masterminds/glide
RUN cd /go/src/github.com/yarpc/yab && glide install
RUN cd /go/src/github.com/yarpc/yab && go install .

ENTRYPOINT ["yab"]

