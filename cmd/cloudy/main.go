package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
)

type RegionsRequest struct {
	Regions []string `json:"regions" binding:"required"`
}

type Resource struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	State      string            `json:"state,omitempty"`
	Region     string            `json:"region"`
	Tags       map[string]string `json:"tags,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type RegionResources struct {
	Region    string     `json:"region"`
	Resources []Resource `json:"resources"`
	Error     string     `json:"error,omitempty"`
}

type ListResourcesResponse struct {
	RegionData []RegionResources `json:"region_data"`
	TotalCount int               `json:"total_count"`
}

type AWSResourceLister struct {
	cfg config.Config
}

func NewAWSResourceLister() (*AWSResourceLister, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	return &AWSResourceLister{cfg: cfg}, nil
}

func (a *AWSResourceLister) ListResourcesInRegion(ctx context.Context, region string) ([]Resource, error) {
	var resources []Resource
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Create region-specific config
	var regionCfg aws.Config
	regionCfg.Region = region

	// Channel to collect errors
	errCh := make(chan error, 6)

	// List EC2 Instances
	wg.Add(1)
	go func() {
		defer wg.Done()
		if ec2Resources, err := a.listEC2Instances(ctx, regionCfg); err != nil {
			errCh <- fmt.Errorf("EC2 instances in %s: %w", region, err)
		} else {
			mu.Lock()
			resources = append(resources, ec2Resources...)
			mu.Unlock()
		}
	}()

	// List S3 Buckets (only in us-east-1 to avoid duplicates)
	if region == "us-east-1" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s3Resources, err := a.listS3Buckets(ctx, regionCfg); err != nil {
				errCh <- fmt.Errorf("S3 buckets: %w", err)
			} else {
				mu.Lock()
				resources = append(resources, s3Resources...)
				mu.Unlock()
			}
		}()
	}

	// List RDS Instances
	wg.Add(1)
	go func() {
		defer wg.Done()
		if rdsResources, err := a.listRDSInstances(ctx, regionCfg); err != nil {
			errCh <- fmt.Errorf("RDS instances in %s: %w", region, err)
		} else {
			mu.Lock()
			resources = append(resources, rdsResources...)
			mu.Unlock()
		}
	}()

	// List Lambda Functions
	wg.Add(1)
	go func() {
		defer wg.Done()
		if lambdaResources, err := a.listLambdaFunctions(ctx, regionCfg); err != nil {
			errCh <- fmt.Errorf("lambda functions in %s: %w", region, err)
		} else {
			mu.Lock()
			resources = append(resources, lambdaResources...)
			mu.Unlock()
		}
	}()

	// List ECS Clusters
	wg.Add(1)
	go func() {
		defer wg.Done()
		if ecsResources, err := a.listECSClusters(ctx, regionCfg); err != nil {
			errCh <- fmt.Errorf("ECS clusters in %s: %w", region, err)
		} else {
			mu.Lock()
			resources = append(resources, ecsResources...)
			mu.Unlock()
		}
	}()

	// List IAM Users (only in us-east-1 to avoid duplicates)
	if region == "us-east-1" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if iamResources, err := a.listIAMUsers(ctx, regionCfg); err != nil {
				errCh <- fmt.Errorf("IAM users: %w", err)
			} else {
				mu.Lock()
				resources = append(resources, iamResources...)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	close(errCh)

	// Collect any errors
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return resources, fmt.Errorf("encountered %d errors while listing resources", len(errors))
	}

	return resources, nil
}

func (a *AWSResourceLister) listEC2Instances(ctx context.Context, cfg aws.Config) ([]Resource, error) {
	client := ec2.NewFromConfig(cfg)
	result, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{})
	if err != nil {
		return nil, err
	}

	var resources []Resource
	for _, reservation := range result.Reservations {
		for _, instance := range reservation.Instances {
			tags := make(map[string]string)
			name := ""
			for _, tag := range instance.Tags {
				if tag.Key != nil && tag.Value != nil {
					tags[*tag.Key] = *tag.Value
					if *tag.Key == "Name" {
						name = *tag.Value
					}
				}
			}

			attributes := map[string]string{
				"instance_type": string(instance.InstanceType),
				"vpc_id":        aws_string_value(instance.VpcId),
				"subnet_id":     aws_string_value(instance.SubnetId),
			}

			if instance.PublicIpAddress != nil {
				attributes["public_ip"] = *instance.PublicIpAddress
			}
			if instance.PrivateIpAddress != nil {
				attributes["private_ip"] = *instance.PrivateIpAddress
			}

			resources = append(resources, Resource{
				ID:         aws_string_value(instance.InstanceId),
				Name:       name,
				Type:       "EC2 Instance",
				State:      string(instance.State.Name),
				Region:     cfg.Region,
				Tags:       tags,
				Attributes: attributes,
			})
		}
	}

	return resources, nil
}

func (a *AWSResourceLister) listS3Buckets(ctx context.Context, cfg aws.Config) ([]Resource, error) {
	client := s3.NewFromConfig(cfg)
	result, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}

	var resources []Resource
	for _, bucket := range result.Buckets {
		resources = append(resources, Resource{
			ID:     aws_string_value(bucket.Name),
			Name:   aws_string_value(bucket.Name),
			Type:   "S3 Bucket",
			Region: "global", // S3 buckets are global but shown in us-east-1
			Attributes: map[string]string{
				"created": bucket.CreationDate.String(),
			},
		})
	}

	return resources, nil
}

func (a *AWSResourceLister) listRDSInstances(ctx context.Context, cfg aws.Config) ([]Resource, error) {
	client := rds.NewFromConfig(cfg)
	result, err := client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{})
	if err != nil {
		return nil, err
	}

	var resources []Resource
	for _, instance := range result.DBInstances {
		attributes := map[string]string{
			"engine":         aws_string_value(instance.Engine),
			"engine_version": aws_string_value(instance.EngineVersion),
			"instance_class": aws_string_value(instance.DBInstanceClass),
		}

		if instance.Endpoint != nil {
			attributes["endpoint"] = aws_string_value(instance.Endpoint.Address)
			if instance.Endpoint.Port != nil {
				attributes["port"] = fmt.Sprintf("%d", *instance.Endpoint.Port)
			}
		}

		resources = append(resources, Resource{
			ID:         aws_string_value(instance.DBInstanceIdentifier),
			Name:       aws_string_value(instance.DBInstanceIdentifier),
			Type:       "RDS Instance",
			State:      aws_string_value(instance.DBInstanceStatus),
			Region:     cfg.Region,
			Attributes: attributes,
		})
	}

	return resources, nil
}

func (a *AWSResourceLister) listLambdaFunctions(ctx context.Context, cfg aws.Config) ([]Resource, error) {
	client := lambda.NewFromConfig(cfg)
	result, err := client.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	if err != nil {
		return nil, err
	}

	var resources []Resource
	for _, function := range result.Functions {
		attributes := map[string]string{
			"runtime":     string(function.Runtime),
			"handler":     aws_string_value(function.Handler),
			"memory_size": fmt.Sprintf("%d", aws_int32_value(function.MemorySize)),
			"timeout":     fmt.Sprintf("%d", aws_int32_value(function.Timeout)),
		}

		resources = append(resources, Resource{
			ID:         aws_string_value(function.FunctionArn),
			Name:       aws_string_value(function.FunctionName),
			Type:       "Lambda Function",
			State:      string(function.State),
			Region:     cfg.Region,
			Attributes: attributes,
		})
	}

	return resources, nil
}

func (a *AWSResourceLister) listECSClusters(ctx context.Context, cfg aws.Config) ([]Resource, error) {
	client := ecs.NewFromConfig(cfg)
	listResult, err := client.ListClusters(ctx, &ecs.ListClustersInput{})
	if err != nil {
		return nil, err
	}

	if len(listResult.ClusterArns) == 0 {
		return []Resource{}, nil
	}

	describeResult, err := client.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: listResult.ClusterArns,
	})
	if err != nil {
		return nil, err
	}

	var resources []Resource
	for _, cluster := range describeResult.Clusters {
		attributes := map[string]string{
			"active_services_count": fmt.Sprintf("%d", cluster.ActiveServicesCount),
			"running_tasks_count":   fmt.Sprintf("%d", cluster.RunningTasksCount),
			"pending_tasks_count":   fmt.Sprintf("%d", cluster.PendingTasksCount),
		}

		resources = append(resources, Resource{
			ID:         aws_string_value(cluster.ClusterArn),
			Name:       aws_string_value(cluster.ClusterName),
			Type:       "ECS Cluster",
			State:      aws_string_value(cluster.Status),
			Region:     cfg.Region,
			Attributes: attributes,
		})
	}

	return resources, nil
}

func (a *AWSResourceLister) listIAMUsers(ctx context.Context, cfg aws.Config) ([]Resource, error) {
	client := iam.NewFromConfig(cfg)
	result, err := client.ListUsers(ctx, &iam.ListUsersInput{})
	if err != nil {
		return nil, err
	}

	var resources []Resource
	for _, user := range result.Users {
		attributes := map[string]string{
			"path":    aws_string_value(user.Path),
			"created": user.CreateDate.String(),
			"user_id": aws_string_value(user.UserId),
		}

		resources = append(resources, Resource{
			ID:         aws_string_value(user.Arn),
			Name:       aws_string_value(user.UserName),
			Type:       "IAM User",
			Region:     "global", // IAM is global
			Attributes: attributes,
		})
	}

	return resources, nil
}

// Helper functions
func aws_string_value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func aws_int32_value(i *int32) int32 {
	if i == nil {
		return 0
	}
	return *i
}

func listResources(c *gin.Context) {
	var req RegionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Regions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one region must be specified"})
		return
	}

	lister, err := NewAWSResourceLister()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize AWS client: " + err.Error()})
		return
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var regionData []RegionResources

	for _, region := range req.Regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()

			resources, err := lister.ListResourcesInRegion(ctx, r)

			mu.Lock()
			if err != nil {
				regionData = append(regionData, RegionResources{
					Region:    r,
					Resources: resources, // Include partial results even with errors
					Error:     err.Error(),
				})
			} else {
				regionData = append(regionData, RegionResources{
					Region:    r,
					Resources: resources,
				})
			}
			mu.Unlock()
		}(region)
	}

	wg.Wait()

	// Calculate total count
	totalCount := 0
	for _, rd := range regionData {
		totalCount += len(rd.Resources)
	}

	response := ListResourcesResponse{
		RegionData: regionData,
		TotalCount: totalCount,
	}

	c.JSON(http.StatusOK, response)
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "cloudy",
		"version": "1.0.0",
	})
}

func setupRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// Add CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Header("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Routes
	r.GET("/health", healthCheck)
	r.POST("/api/v1/resources", listResources)

	return r
}

func main() {
	r := setupRouter()

	log.Println("Starting Cloudy AWS Resource Lister on port 8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
