FROM jenkins/inbound-agent:latest-jdk21

USER root

RUN curl -fsSL https://go.dev/dl/go1.23.4.linux-$(dpkg --print-architecture).tar.gz -o /tmp/go.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"

USER jenkins
