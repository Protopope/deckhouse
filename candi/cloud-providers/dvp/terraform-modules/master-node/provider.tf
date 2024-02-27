terraform {
  # required_version = ">= 0.13.4"
  required_providers {
    kubernetes = {
      source = "hashicorp/kubernetes"
    }
  }
}


provider "kubernetes" {
  config_path = "./config"
  #kubeconfig_base64 = var.cfg
}
