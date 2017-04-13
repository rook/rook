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

x`
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