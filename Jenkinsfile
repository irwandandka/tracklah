// Root-level Jenkinsfile for the infra-tier Jenkins (jenkins.lan), covering
// the whole polyglot repo. Not to be confused with services/location/Jenkinsfile,
// which targets tracklah's own (unused) embedded Jenkins - left untouched.
pipeline {
    agent any
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
                }
                dir('services/trip-events-consumer') {
                    sh 'go build ./...'
                    sh 'go vet ./...'
                }
            }
        }
    }
}
