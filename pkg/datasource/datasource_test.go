package datasource_test

import (
	"context"
  "encoding/json"
  "github.com/aws/aws-sdk-go/aws"
  "github.com/aws/aws-sdk-go/aws/request"
  "github.com/aws/aws-sdk-go/service/xray"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/x-ray-datasource/pkg/datasource"
  "github.com/stretchr/testify/require"
  "testing"
  "time"
)

type XrayClientMock struct{}

func (client *XrayClientMock) GetTraceSummariesPages(input *xray.GetTraceSummariesInput, fn func(*xray.GetTraceSummariesOutput, bool) bool) error {
  http := &xray.Http{
    ClientIp:   aws.String("127.0.0.1"),
    HttpMethod: aws.String("GET"),
    HttpStatus: aws.Int64(200),
    HttpURL:    aws.String("localhost"),
    UserAgent:  nil,
  }

  annotations := make(map[string][]*xray.ValueWithServiceIds)
  annotations["foo"] = []*xray.ValueWithServiceIds{{
    AnnotationValue: &xray.AnnotationValue{},
    ServiceIds:      []*xray.ServiceId{},
  }, {
    AnnotationValue: &xray.AnnotationValue{},
    ServiceIds:      []*xray.ServiceId{},
  }}

  annotations["bar"] = []*xray.ValueWithServiceIds{{
    AnnotationValue: &xray.AnnotationValue{},
    ServiceIds:      []*xray.ServiceId{},
  }}

  summary := &xray.TraceSummary{
    Annotations:            annotations,
    AvailabilityZones:      nil,
    Duration:               aws.Float64(10.5),
    EntryPoint:             nil,
    ErrorRootCauses:        nil,
    FaultRootCauses:        nil,
    HasError:               nil,
    HasFault:               nil,
    HasThrottle:            nil,
    Http:                   http,
    Id:                     aws.String("id1"),
    InstanceIds:            nil,
    IsPartial:              nil,
    MatchedEventTime:       nil,
    ResourceARNs:           nil,
    ResponseTime:           nil,
    ResponseTimeRootCauses: nil,
    Revision:               nil,
    ServiceIds:             nil,
    Users:                  nil,
  }

  output := &xray.GetTraceSummariesOutput{
    ApproximateTime:      aws.Time(time.Now()),
    NextToken:            nil,
    TraceSummaries:       []*xray.TraceSummary{summary},
    TracesProcessedCount: nil,
  }
  fn(output, true)

  return nil
}

func (client *XrayClientMock) BatchGetTraces(input *xray.BatchGetTracesInput) (*xray.BatchGetTracesOutput, error) {
	return &xray.BatchGetTracesOutput{
	  Traces: []*xray.Trace{{
	    Duration: aws.Float64(1.0),
	    Id: aws.String("trace1"),
	    Segments: []*xray.Segment{
	      {
	        Id: aws.String("segment1"),
	        Document: aws.String("{}"),
        },
      },
    }},
  }, nil
}



func (client *XrayClientMock) GetTimeSeriesServiceStatisticsPagesWithContext(context aws.Context, input *xray.GetTimeSeriesServiceStatisticsInput, fn func(*xray.GetTimeSeriesServiceStatisticsOutput, bool) bool, options ...request.Option) error {
  output := &xray.GetTimeSeriesServiceStatisticsOutput{
    TimeSeriesServiceStatistics: []*xray.TimeSeriesServiceStatistics{
      makeTimeSeriesRow(0),
      makeTimeSeriesRow(1),
    },
  }
  fn(output, false)
  return nil
}

func makeTimeSeriesRow(index int) *xray.TimeSeriesServiceStatistics {
  return &xray.TimeSeriesServiceStatistics{
    EdgeSummaryStatistics: &xray.EdgeStatistics{
      ErrorStatistics: &xray.ErrorStatistics{
        OtherCount:    aws.Int64(10),
        ThrottleCount: aws.Int64(10),
        TotalCount:    aws.Int64(20),
      },
      FaultStatistics: &xray.FaultStatistics{
        OtherCount: aws.Int64(15),
        TotalCount: aws.Int64(20),
      },
      OkCount:           aws.Int64(40),
      TotalCount:        aws.Int64(80),
      TotalResponseTime: aws.Float64(3.14),
    },
    ResponseTimeHistogram: []*xray.HistogramEntry{
      {
        Count: aws.Int64(5),
        Value: aws.Float64(42.42),
      },
    },
    ServiceSummaryStatistics: nil,
    Timestamp:                aws.Time(time.Date(2020, 6, 20, 1, index, 1, 0, time.UTC)),
  }
}

func clientFactory(pluginContext *backend.PluginContext) (datasource.XrayClient, error) {
	return &XrayClientMock{}, nil
}

func TestDatasource(t *testing.T) {
	ds := datasource.NewDatasource(clientFactory)

	t.Run("getTrace query", func(t *testing.T) {
	  queryData := datasource.GetTraceQueryData{
	    Query: "traceID",
    }
    jsonData, _ := json.Marshal(queryData)

		response, err := ds.QueryMux.QueryData(
		  context.Background(),
		  &backend.QueryDataRequest{Queries: []backend.DataQuery{{ RefID: "A", QueryType: datasource.QueryGetTrace, JSON: jsonData }}},
    )
		require.NoError(t, err)
    require.NoError(t, response.Responses["A"].Error)

    require.Equal(t, 1, response.Responses["A"].Frames[0].Fields[0].Len())
    require.JSONEq(
      t,
      "{\"Duration\":1,\"Id\":\"trace1\",\"Segments\":[{\"Document\":\"{}\",\"Id\":\"segment1\"}]}",
      response.Responses["A"].Frames[0].Fields[0].At(0).(string),
    )
	})

  t.Run("getTimeSeriesServiceStatistics query", func(t *testing.T) {
    queryData := datasource.GetTraceQueryData{
      Query: "traceID",
    }
    jsonData, _ := json.Marshal(queryData)

    response, err := ds.QueryMux.QueryData(
      context.Background(),
      &backend.QueryDataRequest{
        Queries: []backend.DataQuery{{
          RefID: "A",
          QueryType: datasource.QueryGetTimeSeriesServiceStatistics,
          JSON: jsonData,
        }},
      },
    )
    require.NoError(t, err)
    require.NoError(t, response.Responses["A"].Error)

    require.Equal(t, 2, response.Responses["A"].Frames[0].Fields[0].Len())
    require.Equal(t, 8, len(response.Responses["A"].Frames))
    require.Equal(t, "Time", response.Responses["A"].Frames[0].Fields[0].Name)
    require.Equal(t, "ErrorStatistics.OtherCount", response.Responses["A"].Frames[0].Fields[1].Name)
    require.Equal(
      t,
      time.Date(2020, 6, 20, 1, 0, 1, 0, time.UTC).String(),
      response.Responses["A"].Frames[0].Fields[0].At(0).(*time.Time).String(),
    )
    require.Equal(t, int64(10), response.Responses["A"].Frames[0].Fields[1].At(0))
  })

  t.Run("getTraceSummaries query", func(t *testing.T) {
    queryData := datasource.GetTraceSummariesQueryData{
      Query: "",
    }
    jsonData, _ := json.Marshal(queryData)

    response, err := ds.QueryMux.QueryData(
      context.Background(),
      &backend.QueryDataRequest{Queries: []backend.DataQuery{{ RefID: "A", QueryType: datasource.QueryGetTraceSummaries, JSON: jsonData }}},
    )
    require.NoError(t, err)
    require.NoError(t, response.Responses["A"].Error)

    frame := response.Responses["A"].Frames[0]
    require.Equal(t, 1, frame.Fields[0].Len())
    require.Equal(t, "id1", frame.Fields[0].At(0))
    require.Equal(t, "GET", frame.Fields[1].At(0))
    require.Equal(t, 10.5, frame.Fields[3].At(0))
    require.Equal(t, int64(3), frame.Fields[6].At(0))
  })
}