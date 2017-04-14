// Rook build for Jenkins Pipelines

try {
    node("ec2-stateful") {

        def DOWNLOADDIR='~/.download'

        stage('Checkout') {
            echo 'faking a check-out'
            //checkout scm
        }

        stage('Validation') {
            echo 'faking validation'
        }

        stage('Build') {
            echo 'Simulating a build by doing a pull'
            //sh "sudo mkdir -p /to-host"
            //sh "sudo docker pull quay.io/rook/rook-client"
            //sh "sudo docker pull quay.io/rook/rook-operator"
            //sh "sudo docker pull quay.io/rook/rookd"
            //sh 'echo'
        }

        stage('Tests') {
            sh "sudo apt-get install -qy golang-go"
            //sh "sudo mkdir -p ~/go/src"
            //sh "sudo mkdir -p ~/go/bin"

withEnv(["GOPATH=/home/ubuntu/go", "GOROOT=/usr/local/go"]) {

sh "export GOPATH=/home/ubuntu/go"

                 sh "export GOPATH=/home/ubuntu/go && export GOROOT=/usr/local/go && export PATH=$GOPATH/bin:$GOROOT/bin:$PATH && go get -u github.com/jstemmer/go-junit-report"
     echo 'attempting pull of rook stuff'
                 sh "sudo go get -u github.com/dangula/rook"

                 //sh "cd $GOPATH/src/github.com/rook/e2e/tests/integration/smokeTest"

                 //sh "sudo go test -run TestFileStorage_SmokeTest -v | sudo go-junit-report > file-test-report.xml"

    }
            //sh "export $GOPATH=~/go/"
            //sh "export GOROOT=/usr/local/go/"
            //sh "export PATH=$GOPATH/bin:$GOROOT/bin:$PATH"
            //sh "echo $GOPATH"


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