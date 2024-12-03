# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -e

image=$1
repository=$(echo $SKAFFOLD_IMAGE | rev | cut -d':' -f 2- | rev)
tag=$(echo $SKAFFOLD_IMAGE | rev | cut -d':' -f 1 | rev)

cat <<EOF >example/provider-local/garden/operator/patch-imagevector-overwrite-${image}.yaml
apiVersion: operator.gardener.cloud/v1alpha1
kind: Extension
metadata:
  name: provider-local
spec:
  deployment:
    extension:
      values:
        imageVectorOverwrite: |
          images:
          - name: ${image}
            repository: ${repository}
            tag: ${tag}
EOF
