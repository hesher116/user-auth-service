FROM golang:1.22.3-alpine

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
COPY .env ./
RUN go mod download

COPY *.go ./

RUN go build -o /myapp

EXPOSE 8045

CMD [ "/myapp" ]
