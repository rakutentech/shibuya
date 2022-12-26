FROM grafana/grafana:9.3.2
ENV GF_SECURITY_ADMIN_USER=shibuya
ENV GF_SECURITY_ADMIN_PASSWORD=shibuya
ENV GF_AUTH_ANONYMOUS_ENABLED=true
ENV GF_AUTH_ANONYMOUS_ORG_NAME="Main Org."
ENV GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer
COPY ./provisioning /etc/grafana/provisioning
COPY config.ini /etc/grafana/config.ini
COPY ./dashboards /var/lib/grafana/dashboards
COPY --chown=grafana:grafana ./plugins /var/lib/grafana/plugins
ARG CLUSTER=local
COPY datasources/${CLUSTER}.yml /etc/grafana/provisioning/datasources/all.yml