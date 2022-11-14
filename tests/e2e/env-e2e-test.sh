# Copyright © 2022 Dell Inc. or its subsidiaries. All Rights Reserved.
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#      http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# [Optional] ginko options for custom runs
export GINKGO_OPTS="-v"

# [Optional] Path to .kube configuration if it is not in the default location
# export KUBECONFIG=""

# Must supply path to values file if different from testfiles/values.yaml
export VALUES_FILE="testfiles/values.yaml"

# USER MODIFICATION REQUIRED: must supply path to your cert-csi binary
export CERT_CSI="/root/cert-csi"

# [Optional] uncomment any modules you want to test
# export AUTHORIZATION=true
# export REPLICATION=true
# export OBSERVABILITY=true
# export AUTHORIZATIONPROXYSERVER=true

# [Optional] namespace of operator if you deployed it to a namespace diffrent form the one below.
# export OPERATOR_NAMESPACE="dell-csm-operator"

# USER MODIFICATION REQUIRED: must supply path to your karavictl binary
# export KARAVICTL="/root/karavictl"

# Must supply storage details to values file if different from testfiles/values.yaml
# stotage type are like powerscale, powerflex, powermax
# export STORAGE_TYPE="powerscale"
# export END_POINT="1.1.1.1:8080"
# export SYSTEM_ID="xxxxxx"
# export STORAGE_USER="xxxx"
# export STORAGE_PASSWORD="xxxxx"
# export STORAGE_POOL="pool"

# User must add control-plane IP
# export CONTROL_PLANE_IP="1.1.1.1"

# User must specify and create driver namespace
# export DRIVER_NAMESPACE="namespace"
