resource "aws_elasticache_subnet_group" "redis" {
  name       = "${var.app_name}-redis-subnets"
  subnet_ids = [aws_subnet.private_1.id, aws_subnet.private_2.id]
}

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id       = "${var.app_name}-redis-cluster"
  description                = "ElastiCache Redis cluster for NanoLink"
  node_type                  = "cache.r6g.large"
  num_cache_clusters         = 2
  port                       = 6379
  parameter_group_name       = "default.redis7.cluster.on"
  subnet_group_name          = aws_elasticache_subnet_group.redis.name
  automatic_failover_enabled = true
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
}
