output "eks_cluster_name" {
  description = "EKS cluster name"
  value       = module.eks.cluster_name
}

output "eks_cluster_endpoint" {
  description = "EKS cluster API endpoint"
  value       = module.eks.cluster_endpoint
}

output "rds_endpoint" {
  description = "RDS PostgreSQL endpoint"
  value       = aws_db_instance.conduit.endpoint
}

output "rds_database_name" {
  description = "RDS database name"
  value       = aws_db_instance.conduit.db_name
}

output "alb_dns_name" {
  description = "ALB DNS name"
  value       = aws_lb.conduit.dns_name
}

output "vpc_id" {
  description = "VPC ID"
  value       = module.vpc.vpc_id
}

output "database_url" {
  description = "PostgreSQL connection URL (without password)"
  value       = "postgres://conduit:PASSWORD@${aws_db_instance.conduit.endpoint}/conduit?sslmode=require"
  sensitive   = true
}
