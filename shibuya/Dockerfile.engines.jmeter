ARG jmeter_ver=3.3

FROM asia-northeast1-docker.pkg.dev/shibuya-214807/shibuya/alpine:3.10.2 AS jmeter
ARG jmeter_ver
ENV JMETER_VERSION=$jmeter_ver
RUN wget archive.apache.org/dist/jmeter/binaries/apache-jmeter-${JMETER_VERSION}.zip
RUN unzip -qq apache-jmeter-${JMETER_VERSION}

FROM asia-northeast1-docker.pkg.dev/shibuya-214807/shibuya/openjdk:8u212-jdk
ARG GROUP=shibuya
ARG USER=shibuya-agent
ENV AGENT_ROOT=/shibuya-agent
RUN groupadd -r $GROUP && useradd -r -g $GROUP -m -d /home/$USER -s /bin/bash $USER
RUN mkdir -p $AGENT_ROOT && chown -R $USER:$GROUP $AGENT_ROOT
RUN mkdir /test-data && chown -R $USER:$GROUP /test-data

USER ${USER}

ARG jmeter_ver
ENV JMETER_VERSION=$jmeter_ver
RUN mkdir $AGENT_ROOT/test-conf $AGENT_ROOT/test-result
COPY --from=jmeter /apache-jmeter-${JMETER_VERSION} $AGENT_ROOT/apache-jmeter-${JMETER_VERSION}
ADD build/shibuya-agent $AGENT_ROOT/shibuya-agent
ADD engines/jmeter/shibuya.properties $AGENT_ROOT/test-conf/shibuya.properties
ADD engines/jmeter/jmeter.sh $AGENT_ROOT/apache-jmeter-${JMETER_VERSION}/bin/jmeter

CMD ["sh", "-c", "$AGENT_ROOT/shibuya-agent"]
