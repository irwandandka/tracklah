// Root-level Jenkinsfile for the infra-tier Jenkins (jenkins.lan), covering
// the whole polyglot repo. Not to be confused with services/location/Jenkinsfile,
// which targets tracklah's own (unused) embedded Jenkins - left untouched.
//
// Needs on jenkins.lan: a "Username with password" credential with id
// "ghcr-credentials" (GitHub username + classic PAT with write:packages).
pipeline {
    agent any
    environment {
        REGISTRY = 'ghcr.io'
        IMAGE_PREFIX = 'ghcr.io/irwandandka/tracklah'
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
    }
}
