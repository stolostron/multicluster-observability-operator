pipeline {
    options {
        buildDiscarder(logRotator(daysToKeepStr: '30'))
    }
    agent {
        docker {
	    image 'quay.io/vboulos/acmqe-automation/go:go1.21-ginkgo2.17.1'
            args '--network host -u 0:0'
        }
    }
    parameters {
        string(name:'CLOUD_PROVIDER', defaultValue: '', description: 'Cloud provider, OCP and ACM version information, like VMWARE-412-264, AWS-411')
        string(name:'HUB_CLUSTER_NAME', defaultValue: '', description: 'Name of Hub cluster')
        string(name:'BASE_DOMAIN', defaultValue: '', description: 'Base domain of Hub cluster')
        string(name:'OC_CLUSTER_USER', defaultValue: 'kubeadmin', description: 'OCP Hub User Name')
        string(name:'OC_HUB_CLUSTER_PASS', defaultValue: '', description: 'OCP Hub Password')
        string(name:'OC_HUB_CLUSTER_API_URL', defaultValue: '', description: 'OCP Hub API URL')
        string(name:'MANAGED_CLUSTER_NAME', defaultValue: '', description: 'Managed cluster name')
        string(name:'MANAGED_CLUSTER_BASE_DOMAIN', defaultValue: '', description: 'Managed cluster base domain')
        string(name:'MANAGED_CLUSTER_USER', defaultValue: 'kubeadmin', description: 'Managed Cluster User Name')
        string(name:'MANAGED_CLUSTER_PASS', defaultValue: '', description: 'Managed cluster Password')
        string(name:'MANAGED_CLUSTER_API_URL', defaultValue: '', description: 'Managed cluster API URL')
        string(name:'BUCKET', defaultValue: 'obs-auto-bucket', description: 'Bucket name')
        string(name:'REGION', defaultValue: 'us-east-1', description: 'Bucket region')
        password(name:'AWS_ACCESS_KEY_ID', defaultValue: '', description: 'AWS access key ID')
        password(name:'AWS_SECRET_ACCESS_KEY', defaultValue: '', description: 'AWS secret access key')
        string(name:'SKIP_INSTALL_STEP', defaultValue: 'false', description: 'Skip Observability installation')
        string(name:'SKIP_UNINSTALL_STEP', defaultValue: 'true', description: 'Skip Observability uninstallation')
        string(name:'TAGGING', defaultValue: '', description: 'with tagging value to run the specific test cases')
        string(name:'USE_MINIO', defaultValue: 'false', description: 'If no AWS S3 bucket, you could use minio as object storage to instead')
    }
    environment {
        CI = 'true'
        AWS_SECRET_ACCESS_KEY = credentials('cqu_aws_secret_access_key')  
        AWS_ACCESS_KEY_ID = credentials('cqu_aws_access_key')
    }
    stages {
        stage('Test Run') {
            steps {
                sh """
                export CLOUD_PROVIDER="${params.CLOUD_PROVIDER}"
                export OC_CLUSTER_USER="${params.OC_CLUSTER_USER}"
                set +x
                export OC_HUB_CLUSTER_PASS="${params.OC_HUB_CLUSTER_PASS}"
                set -x
                export OC_HUB_CLUSTER_API_URL="${params.OC_HUB_CLUSTER_API_URL}"
                BASE_DOMAIN=\$(echo \${OC_HUB_CLUSTER_API_URL} | awk -F'https://|:' '{gsub(/^api\\./, "", \$2); print \$2}')
                HUB_CLUSTER_NAME=\$(echo \$BASE_DOMAIN | cut -d'.' -f1)
                echo "BASE_DOMAIN: \$BASE_DOMAIN"
                echo "HUB_CLUSTER_NAME: \$HUB_CLUSTER_NAME"
                export HUB_CLUSTER_NAME="\$HUB_CLUSTER_NAME"
                export BASE_DOMAIN="\$BASE_DOMAIN"
                export MANAGED_CLUSTER_NAME="${params.MANAGED_CLUSTER_NAME}"
                export MANAGED_CLUSTER_BASE_DOMAIN="${params.MANAGED_CLUSTER_BASE_DOMAIN}"
                export MANAGED_CLUSTER_USER="${params.MANAGED_CLUSTER_USER}"
                set +x
                export MANAGED_CLUSTER_PASS="${params.MANAGED_CLUSTER_PASS}"
                set -x
                export MANAGED_CLUSTER_API_URL="${params.MANAGED_CLUSTER_API_URL}"
                export BUCKET="${params.BUCKET}"
                export REGION="${params.REGION}"
                export SKIP_INSTALL_STEP="${params.SKIP_INSTALL_STEP}"
                export SKIP_UNINSTALL_STEP="${params.SKIP_UNINSTALL_STEP}"
                export TAGGING="${params.TAGGING}"
                export USE_MINIO="${params.USE_MINIO}"
                export IS_CANARY_ENV=true
                
                if [[ -n "${params.AWS_ACCESS_KEY_ID}" ]]; then
                    export AWS_ACCESS_KEY_ID="${params.AWS_ACCESS_KEY_ID}"
                fi
                
                if [[ -n "${params.AWS_SECRET_ACCESS_KEY}" ]]; then
                    export AWS_SECRET_ACCESS_KEY="${params.AWS_SECRET_ACCESS_KEY}"
                fi
                
                if [[ "${params.USE_MINIO}" == true ]]; then
                  export IS_CANARY_ENV=false
                fi                 
                if [[ -z "${OC_CLUSTER_USER}" || -z "${OC_HUB_CLUSTER_PASS}" || -z "${OC_HUB_CLUSTER_API_URL}" ]]; then
                    echo "Aborting test.. OCP HUB details are required for the test execution"
                    exit 1
                else
		    if [[ -n "${params.MANAGED_CLUSTER_USER}" && -n "${params.MANAGED_CLUSTER_PASS}" && -n "${params.MANAGED_CLUSTER_API_URL}" ]]; then
                      set +x
                      oc login --insecure-skip-tls-verify -u \$MANAGED_CLUSTER_USER -p \$MANAGED_CLUSTER_PASS \$MANAGED_CLUSTER_API_URL
                      set -x
                      oc config view --minify --raw=true > ~/.kube/managed_kubeconfig
                      export MAKUBECONFIG=~/.kube/managed_kubeconfig
                    fi
                    set +x
                    oc login --insecure-skip-tls-verify -u \$OC_CLUSTER_USER -p \$OC_HUB_CLUSTER_PASS \$OC_HUB_CLUSTER_API_URL
                    set -x
                    export KUBECONFIG=~/.kube/config
                    go mod vendor && ginkgo build ./tests/pkg/tests/
                    rm -rf tests/pkg/tests/*.xml
                    cd tests
                    cp resources/options.yaml.template resources/options.yaml
                    /usr/local/bin/yq e -i '.options.hub.name="'"\$HUB_CLUSTER_NAME"'"' resources/options.yaml
                    /usr/local/bin/yq e -i '.options.hub.baseDomain="'"\$BASE_DOMAIN"'"' resources/options.yaml
                    /usr/local/bin/yq e -i '.options.clusters.name="'"\$MANAGED_CLUSTER_NAME"'"' resources/options.yaml
                    /usr/local/bin/yq e -i '.options.clusters.baseDomain="'"\$MANAGED_CLUSTER_BASE_DOMAIN"'"' resources/options.yaml
                    /usr/local/bin/yq e -i '.options.clusters.kubeconfig="'"\$MAKUBECONFIG"'"' resources/options.yaml
                    cat resources/options.yaml
                    if [[ -n "${params.TAGGING}" ]]; then
                    ginkgo --junit-report=./tests/pkg/tests/results.xml --focus="\$TAGGING" -v pkg/tests/ -- -options=../../resources/options.yaml -v=5
                    else
                    ginkgo --junit-report=./tests/pkg/tests/results.xml -v pkg/tests/ -- -options=../../resources/options.yaml -v=5
                    fi
                fi
                """
            }
        }


    }
    post {
        always {
            archiveArtifacts artifacts: 'tests/pkg/tests/*.xml', followSymlinks: false
            junit 'tests/pkg/tests/*.xml'
        }
    }
}
