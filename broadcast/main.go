package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/sns"

	"github.com/aws/aws-sdk-go/service/dynamodb"
)

// Response is of type APIGatewayProxyResponse since we're leveraging the
// AWS Lambda Proxy Request functionality (default behavior)
type Response events.APIGatewayProxyResponse
type Request events.APIGatewayProxyRequest

type Item struct {
	ID      int
	Date    string
	Message string
}

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(request Request) (Response, error) {
	topicArn := os.Getenv("snsTopicArn")
	region := os.Getenv("region")
	tableName := os.Getenv("tableName")

	// Handle Request
	var buf bytes.Buffer

	body, err := json.Marshal(map[string]interface{}{
		"message": request.Body,
	})
	if err != nil {
		return Response{StatusCode: 404}, err
	}

	json.HTMLEscape(&buf, body)

	// AWS Session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})

	if err != nil {
		fmt.Println("NewSession error:", err)
		return Response{StatusCode: 500}, err
	}

	// SNS
	clientSns := sns.New(sess)
	input := &sns.PublishInput{
		Message:  aws.String(buf.String()),
		TopicArn: aws.String(topicArn),
	}

	result, err := clientSns.Publish(input)
	if err != nil {
		fmt.Println("Publish error:", err)
		return Response{StatusCode: 500}, err
	}

	// DynamoDB
	clientDynamodb := dynamodb.New(sess)

	// Create Table
	tableInput := &dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("ID"),
				AttributeType: aws.String("N"),
			},
			{
				AttributeName: aws.String("Date"),
				AttributeType: aws.String("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("ID"),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String("Date"),
				KeyType:       aws.String("RANGE"),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(10),
			WriteCapacityUnits: aws.Int64(10),
		},
		TableName: aws.String(tableName),
	}

	_, err = clientDynamodb.CreateTable(tableInput)
	if err != nil {
		fmt.Println("Got error calling CreateTable:")
		fmt.Println(err.Error())
	} else {
		fmt.Println("Created the table", tableName)
		time.Sleep(5 * time.Second) // give some time for the table to be created
	}
	now := time.Now()
	secs := now.Unix()
	date := time.Unix(secs, 0)

	// Insert Item into Table
	item := Item{
		ID:      int(secs),
		Date:    date.String(),
		Message: buf.String(),
	}

	av, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		fmt.Println("Got error marshalling new movie item:")
		fmt.Println(err.Error())
		return Response{StatusCode: 500}, err
	}

	inputd := &dynamodb.PutItemInput{
		Item:      av,
		TableName: aws.String(tableName),
	}

	_, err = clientDynamodb.PutItem(inputd)
	if err != nil {
		fmt.Println("Got error calling PutItem:")
		fmt.Println(err.Error())
		return Response{StatusCode: 500}, err
	}

	fmt.Println(result)

	// Send Response
	resp := Response{
		StatusCode:      200,
		IsBase64Encoded: false,
		Body:            buf.String(),
		Headers: map[string]string{
			"Content-Type":           "application/json",
			"X-MyCompany-Func-Reply": "broadcast-handler",
		},
	}

	return resp, nil
}

func main() {
	lambda.Start(Handler)
}
