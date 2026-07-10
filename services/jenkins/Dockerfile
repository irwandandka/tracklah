FROM jenkins/jenkins:lts

USER root

# Go toolchain lives on the controller image ONLY as a temporary
# simplification, since there's no separate Build Agent yet (roadmap
# step 4). Best practice is "controller orchestrates, agent builds" -
# this gets revisited once a real agent is wired in.
RUN curl -fsSL https://go.dev/dl/go1.23.4.linux-$(dpkg --print-architecture | sed 's/amd64/amd64/;s/arm64/arm64/').tar.gz -o /tmp/go.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"

USER jenkins

COPY plugins.txt /usr/share/jenkins/ref/plugins.txt
RUN jenkins-plugin-cli --plugin-file /usr/share/jenkins/ref/plugins.txt

COPY casc.yaml /var/jenkins_home/casc.yaml
ENV CASC_JENKINS_CONFIG=/var/jenkins_home/casc.yaml
ENV JAVA_OPTS="-Djenkins.install.runSetupWizard=false"
