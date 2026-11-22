// Root-level Jenkinsfile for the infra-tier Jenkins (jenkins.lan), covering
// the whole polyglot repo. Not to be confused with services/location/Jenkinsfile,
// which targets tracklah's own (unused) embedded Jenkins - left untouched.
//
// Needs on jenkins.lan:
// - "ghcr-credentials": Username with password (GitHub username + classic
//   PAT with write:packages).
// - "tracklah-deploy-key": SSH Username with private key for deploy@vm-4.
//   On vm-4 that key is locked to /opt/tracklah/deploy.sh (see deploy/).
pipeline {
    agent any
    environment {
        REGISTRY = 'ghcr.io'
        IMAGE_PREFIX = 'ghcr.io/irwandandka/tracklah'
        DEPLOY_HOST = 'deploy@192.168.100.163'
    }
    stages {
        stage('api (NestJS)') {
            agent { docker { image 'node:22'; reuseNode true } }
            steps {
                dir('services/api') {
                    sh 'npm ci'
                    sh 'npm run lint'
                    sh 'npm run build'
                    sh 'npm run test'
                }
            }
        }
        stage('Go services') {
            agent { docker { image 'golang:1.23'; reuseNode true } }
            environment {
                // The docker agent runs as uid 1000 (not root), which
                // can't write to the image's default /.cache or /go -
                // point Go's caches at the writable workspace instead.
                GOCACHE = "${WORKSPACE}/.gocache"
                GOPATH = "${WORKSPACE}/.gopath"
            }
            steps {
                dir('services/location') {
                    sh 'go build ./...'
                    sh 'go vet ./...'
                }
                dir('services/driver-simulator') {
                    sh 'go build ./...'
                    sh 'go vet ./...'
                    sh 'go test ./...'
                }
                dir('services/trip-events-consumer') {
                    sh 'go build ./...'
                    sh 'go vet ./...'
                }
            }
        }
        // Only main publishes images, and only after every check above
        // passed. Works for both multibranch jobs (BRANCH_NAME=main) and
        // plain pipeline jobs (GIT_BRANCH=origin/main).
        stage('Docker build & push (GHCR)') {
            when {
                expression {
                    def branch = env.BRANCH_NAME ?: env.GIT_BRANCH ?: ''
                    return branch.replaceFirst('^origin/', '') == 'main'
                }
            }
            steps {
                script {
                    // Immutable tag per commit for deploys/rollbacks, plus
                    // "latest" as a moving pointer.
                    def tag = env.GIT_COMMIT.substring(0, 7)
                    def labels = "--label org.opencontainers.image.source=https://github.com/irwandandka/tracklah " +
                                 "--label org.opencontainers.image.revision=${env.GIT_COMMIT}"
                    def services = ['api', 'location', 'driver-simulator', 'trip-events-consumer']
                    docker.withRegistry("https://${env.REGISTRY}", 'ghcr-credentials') {
                        for (svc in services) {
                            def img = docker.build("${env.IMAGE_PREFIX}-${svc}:${tag}", "--pull ${labels} services/${svc}")
                            img.push()
                            img.push('latest')
                        }
                    }
                }
            }
        }
        // Deploys the images just pushed to vm-4. The deploy key can only
        // run deploy.sh there (forced command), which pulls the given tag,
        // waits for health checks, and rolls back if they fail. The host
        // key is pinned in deploy/known_hosts.
        stage('Deploy to vm-4') {
            when {
                expression {
                    def branch = env.BRANCH_NAME ?: env.GIT_BRANCH ?: ''
                    return branch.replaceFirst('^origin/', '') == 'main'
                }
            }
            steps {
                script {
                    env.RELEASE_TAG = env.GIT_COMMIT.substring(0, 7)
                }
                withCredentials([sshUserPrivateKey(credentialsId: 'tracklah-deploy-key', keyFileVariable: 'DEPLOY_KEY')]) {
                    sh '''
                        ssh -i "$DEPLOY_KEY" -o IdentitiesOnly=yes \
                            -o StrictHostKeyChecking=yes -o UserKnownHostsFile=deploy/known_hosts \
                            "$DEPLOY_HOST" "$RELEASE_TAG" < deploy/docker-compose.yml
                    '''
                }
            }
        }
    }
}
