ARG jmeter_ver=3.3

FROM gcr.io/shibuya-214807/alpine:3.10.2 AS jmeter
ARG jmeter_ver
ENV JMETER_VERSION=$jmeter_ver
RUN wget https://archive.apache.org/dist/jmeter/binaries/apache-jmeter-${JMETER_VERSION}.zip
RUN unzip -qq apache-jmeter-${JMETER_VERSION}

FROM gcr.io/shibuya-214807/golang:1.13.6-stretch AS shibuya-agent
RUN go get github.com/hpcloud/tail
ADD shibuya-agent.go /go/src/shibuya-agent/shibuya-agent.go
WORKDIR /go/src/shibuya-agent
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build  -o /go/bin/shibuya-agent

FROM gcr.io/shibuya-214807/openjdk:8u212-jdk
ARG jmeter_ver
ENV JMETER_VERSION=$jmeter_ver
RUN mkdir /test-conf /test-result
COPY --from=jmeter /apache-jmeter-${JMETER_VERSION} /apache-jmeter-${JMETER_VERSION}
COPY --from=shibuya-agent /go/bin/shibuya-agent /usr/local/bin/shibuya-agent
ADD shibuya.properties /test-conf/shibuya.properties
ADD jmeter.sh /apache-jmeter-${JMETER_VERSION}/bin/jmeter
CMD ["shibuya-agent"]