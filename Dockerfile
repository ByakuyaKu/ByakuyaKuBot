FROM golang:1.13.15-buster

RUN mkdir /app

ADD . /app

WORKDIR /app

RUN apt-get update
RUN apt-get install -y git
#RUN GO111MODULE = 'on'
RUN go get github.com/Syfaro/telegram-bot-api
RUN go get github.com/SevereCloud/vksdk/v2@latest
RUN go build -o main .

CMD ["/app/main"]