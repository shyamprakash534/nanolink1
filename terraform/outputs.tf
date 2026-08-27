output "alb_dns_name" {
  value       = aws_lb.main.dns_name
  description = "Application Load Balancer DNS Endpoint"
}

output "rds_endpoint" {
  value       = aws_db_instance.postgres.endpoint
  description = "RDS PostgreSQL Connection Endpoint"
}

output "redis_primary_endpoint" {
  value       = aws_elasticache_replication_group.redis.primary_endpoint_address
  description = "Redis Primary Node Endpoint"
}
