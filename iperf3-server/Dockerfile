FROM alpine:latest

WORKDIR /iperf

RUN apk add --no-cache --no-check-certificate iperf3
COPY ./serve.sh .
RUN chmod +x serve.sh

ENV PORT="5201"

ENTRYPOINT ["./serve.sh"]
