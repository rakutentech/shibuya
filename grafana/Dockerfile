FROM asia-northeast1-docker.pkg.dev/shibuya-214807/shibuya/grafana:5.3.2
ENV GF_SECURITY_ADMIN_USER=shibuya
ENV GF_SECURITY_ADMIN_PASSWORD=shibuya
ENV GF_AUTH_ANONYMOUS_ENABLED=true
ENV GF_AUTH_ANONYMOUS_ORG_NAME="Main Org."
ENV GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer
COPY ./provisioning /etc/grafana/provisioning
COPY config.ini /etc/grafana/config.ini
COPY ./dashboards /var/lib/grafana/dashboards
COPY --chown=grafana:grafana ./plugins /var/lib/grafana/plugins
