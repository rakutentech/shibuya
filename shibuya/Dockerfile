FROM ubuntu:18.04

RUN apt-get update && apt-get install -y curl
RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl \
    && chmod +x ./kubectl \
    && mv ./kubectl /usr/local/bin/kubectl

ARG binary_name=shibuya
ADD ./build/${binary_name} /usr/local/bin/${binary_name}

ENV GOOGLE_APPLICATION_CREDENTIALS /auth/shibuya-gcp.json

ARG env=local
ENV env ${env}

COPY ./ui/ /
ENV binary=${binary_name}
CMD ${binary}
