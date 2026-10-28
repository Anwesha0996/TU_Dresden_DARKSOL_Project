Write-Host "Starting kubectl-DTN workflow..."

$KUBECONFIG_PATH = "C:\Users\HP\talos-vm\kubeconfig"

kubectl --kubeconfig $KUBECONFIG_PATH apply -f app-deployment.yaml

kubectl --kubeconfig $KUBECONFIG_PATH apply -f app-service.yaml

Write-Host "Deployment completed."

kubectl --kubeconfig $KUBECONFIG_PATH get pods
kubectl --kubeconfig $KUBECONFIG_PATH get svc