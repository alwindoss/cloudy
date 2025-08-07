# Cloudy - AWS Resource Lister

A REST API service built with Go and Gin that lists active AWS resources across multiple regions.

## Features

- Lists AWS resources across multiple regions concurrently
- Supports the following AWS services:
  - EC2 Instances
  - S3 Buckets (global, shown in us-east-1)
  - RDS Instances
  - Lambda Functions  
  - ECS Clusters
  - IAM Users (global, shown in us-east-1)
- RESTful API with JSON input/output
- Concurrent processing for better performance
- Health check endpoint
- Docker support

## Prerequisites

- Go 1.21 or later
- AWS credentials configured (via AWS CLI, environment variables, or IAM roles)
- Appropriate AWS permissions to list resources

## Installation

1. Clone the repository:
```bash
git clone https://github.com/alwindoss/cloudy.git
cd cloudy
```

2. Install dependencies:
```bash
go mod tidy
```

3. Run the application:
```bash
go run main.go
```

## AWS Permissions

The application requires the following AWS permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ec2:DescribeInstances",
                "s3:ListBuckets",
                "rds:DescribeDBInstances",
                "lambda:ListFunctions",
                "ecs:ListClusters",
                "ecs:DescribeClusters",
                "iam:ListUsers"
            ],
            "Resource": "*"
        }
    ]
}
```

## API Endpoints

### Health Check
- **GET** `/health`
- Returns service health status

### List Resources
- **POST** `/api/v1/resources`
- Lists AWS resources across specified regions

#### Request Format
```json
{
  "regions": ["us-east-1", "us-west-2", "eu-west-1"]
}
```

#### Response Format
```json
{
  "region_data": [
    {
      "region": "us-east-1",
      "resources": [
        {
          "id": "i-1234567890abcdef0",
          "name": "web-server",
          "type": "EC2 Instance",
          "state": "running",
          "region": "us-east-1",
          "tags": {
            "Name": "web-server",
            "Environment": "production"
          },
          "attributes": {
            "instance_type": "t3.micro",
            "vpc_id": "vpc-12345678",
            "public_ip": "1.2.3.4"
          }
        }
      ],
      "error": ""
    }
  ],
  "total_count": 1
}
```

## Usage Examples

### Using curl
```bash
# List resources in multiple regions
curl -X POST http://localhost:8080/api/v1/resources \
  -H "Content-Type: application/json" \
  -d '{"regions": ["us-east-1", "us-west-2"]}'

# Health check
curl http://localhost:8080/health
```

### Using Docker
```bash
# Build the image
docker build -t cloudy .

# Run with AWS credentials
docker run -p 8080:8080 \
  -e AWS_ACCESS_KEY_ID=your_access_key \
  -e AWS_SECRET_ACCESS_KEY=your_secret_key \
  -e AWS_REGION=us-east-1 \
  cloudy
```

## Configuration

The application uses the AWS SDK's default credential chain:
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
2. AWS credentials file (`~/.aws/credentials`)
3. IAM roles (when running on EC2)
4. AWS SSO

## Development

### Project Structure
```
.
├── main.go          # Main application code
├── go.mod           # Go module definition
├── go.sum           # Go dependencies
├── Dockerfile       # Docker configuration
└── README.md        # This file
```

### Running Tests
```bash
go test ./...
```

### Building for Production
```bash
CGO_ENABLED=0 GOOS=linux go build -o cloudy .
```

## Error Handling

- Individual resource type failures don't stop the entire operation
- Partial results are returned even when some services fail
- Errors are reported per region in the response
- HTTP status codes indicate overall request success/failure

## Performance Considerations

- Concurrent processing of regions for faster response times
- Concurrent processing of different resource types within each region
- Reasonable timeouts for AWS API calls
- Memory-efficient streaming where possible