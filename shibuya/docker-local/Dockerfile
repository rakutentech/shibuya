FROM gcr.io/shibuya-214807/golang:1.13.6-stretch AS builder

RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl \
    && chmod +x ./kubectl \
    && mv ./kubectl /usr/local/bin/kubectl

# Use only binaries from above image for running the app
FROM gcr.io/shibuya-214807/ubuntu:18.04

COPY --from=builder /usr/local/bin/kubectl /usr/local/bin/kubectl
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ADD ./build/shibuya /usr/local/bin/shibuya

RUN mkdir /auth
ADD ./shibuya-gcp.json /auth/shibuya-gcp.json
ENV GOOGLE_APPLICATION_CREDENTIALS /auth/shibuya-gcp.json

ARG env=local
ENV env ${env}
ARG lab_image=""
ENV lab_image ${lab_image}
ARG proxy=""
ENV http_proxy ${proxy}
ENV https_proxy ${proxy}

COPY config/kube_configs /root/.kube
COPY config.json /config.json
COPY ./ui/ /
COPY ./controller/mandatory-1.yaml /ingress/mandatory-1.yaml
CMD ["shibuya"]