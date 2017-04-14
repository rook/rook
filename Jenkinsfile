// Rook build for Jenkins Pipelines

try {
    node("ec2-stateful") {

        def DOWNLOADDIR='~/.download'

        stage('Checkout') {
            echo 'faking a check-out'
            checkout scm
        }

        stage('Validation') {
            echo 'faking validation'
        }

        stage('Build') {
            echo 'Simulating a build by doing a pull'
            sh "sudo mkdir -p /to-host"
            sh "sudo docker pull quay.io/rook/rook-client"
            sh "sudo docker pull quay.io/rook/rook-operator"
            sh "sudo docker pull quay.io/rook/rookd"
        }

        stage('Tests') {
            go get -u github.com/jstemmer/go-junit-report

            cd e2e/tests/integration/smokeTest

            go test -run TestFileStorage_SmokeTest -v | go-junit-report > file-test-report.xml

            step([$class: 'JUnitResultArchiver', testResults: '**/target/surefire-reports/*.xml'])

        }

        stage('Cleanup') {


            deleteDir()
        }
    }
}
catch (Exception e) {
    echo 'Failure encountered'

    node("ec2-stateful") {
        echo "Cleaning docker"

        echo 'Cleaning up workspace'
        deleteDir()
    }

    exit 1
}