resource "aws_db_subnet_group" "rds" {
  name       = "${var.app_name}-rds-subnets"
  subnet_ids = [aws_subnet.private_1.id, aws_subnet.private_2.id]
}

resource "aws_db_instance" "postgres" {
  identifier             = "${var.app_name}-postgres"
  engine                 = "postgres"
  engine_version         = "16.1"
  instance_class         = "db.r6g.xlarge"
  allocated_storage      = 100
  max_allocated_storage  = 1000
  db_name                = "nanolink"
  username               = "nanolink"
  password               = var.db_password
  db_subnet_group_name   = aws_db_subnet_group.rds.name
  multi_az               = true
  skip_final_snapshot    = true
  publicly_accessible    = false
}
