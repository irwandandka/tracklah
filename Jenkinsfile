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
