FROM jenkins/jenkins:lts

USER jenkins

COPY plugins.txt /usr/share/jenkins/ref/plugins.txt
RUN jenkins-plugin-cli --plugin-file /usr/share/jenkins/ref/plugins.txt

# NOTE: this must NOT live under /var/jenkins_home - that path is a
# persistent volume, so anything copied there only "sticks" on the very
# first boot (when the volume is still empty). Every rebuild after that
# would silently be ignored in favor of whatever's already in the volume.
# Keeping it at a separate path guarantees each image rebuild's casc.yaml
# actually gets applied.
COPY casc.yaml /usr/share/jenkins/casc.yaml
ENV CASC_JENKINS_CONFIG=/usr/share/jenkins/casc.yaml
ENV JAVA_OPTS="-Djenkins.install.runSetupWizard=false"
