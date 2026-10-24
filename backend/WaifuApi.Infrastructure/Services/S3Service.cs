using System;
using System.Collections.Generic;
using System.IO;
using System.Net.Http;
using System.Threading;
using System.Threading.Tasks;
using Amazon;
using Amazon.Runtime;
using Amazon.S3;
using Amazon.S3.Model;
using Microsoft.Extensions.Configuration;
using WaifuApi.Application.Interfaces;

namespace WaifuApi.Infrastructure.Services;

public class S3Service : IStorageService
{
    private readonly IAmazonS3 _s3Client;
    private readonly string _bucketName;

    public S3Service(IConfiguration configuration)
    {
        var accessKey = configuration["S3:AccessKey"];
        var secretKey = configuration["S3:SecretKey"];
        var serviceUrl = configuration["S3:ServiceUrl"];
        var region = configuration["S3:Region"];
        _bucketName = configuration["S3:BucketName"]!;
        
        if (string.IsNullOrEmpty(accessKey) || string.IsNullOrEmpty(secretKey) || string.IsNullOrEmpty(serviceUrl) || string.IsNullOrEmpty(_bucketName) || string.IsNullOrEmpty(region))
        {
            throw new InvalidOperationException("S3 configuration is incomplete. AccessKey, SecretKey, ServiceUrl, BucketName, and Region are required.");
        }

        var forcePathStyle = configuration.GetValue<bool>("S3:ForcePathStyle", false);
        var verifySsl = configuration.GetValue<bool>("S3:VerifySsl", true);

        var config = new AmazonS3Config
        {
            ServiceURL = serviceUrl,
            AuthenticationRegion = region,
            ForcePathStyle = forcePathStyle,
            // AWS SDK v4 sends trailing checksums by default (Content-Encoding:
            // STREAMING-AWS4-HMAC-SHA256-PAYLOAD-TRAILER), which S3-compatible
            // stores like Cloudflare R2 reject with "not implemented". Only
            // request/validate checksums when an operation strictly requires them.
            RequestChecksumCalculation = RequestChecksumCalculation.WHEN_REQUIRED,
            ResponseChecksumValidation = ResponseChecksumValidation.WHEN_REQUIRED,
        };

        if (!verifySsl)
        {
            config.HttpClientFactory = new UntrustedCertClientFactory();
        }

        _s3Client = new AmazonS3Client(accessKey, secretKey, config);
    }

    public async Task UploadAsync(Stream stream, string fileName, string contentType)
    {
        var request = new PutObjectRequest
        {
            BucketName = _bucketName,
            Key = fileName,
            InputStream = stream,
            ContentType = contentType,
            AutoCloseStream = false,
            CannedACL = S3CannedACL.PublicRead,
            // Cloudflare R2 does not support the SDK's streaming SigV4 upload
            // (aws-chunked). Disable payload signing so the request goes out
            // with Content-Length + UNSIGNED-PAYLOAD, which R2 supports.
            DisablePayloadSigning = true,
            DisableDefaultChecksumValidation = true,
        };

        await _s3Client.PutObjectAsync(request);
    }

    public async Task DeleteAsync(string fileName)
    {
        await _s3Client.DeleteObjectAsync(_bucketName, fileName);
    }

    public async Task<List<string>> ListObjectsAsync(CancellationToken ct)
    {
        var keys = new List<string>();
        string? continuationToken = null;

        do
        {
            var request = new ListObjectsV2Request
            {
                BucketName = _bucketName,
                ContinuationToken = continuationToken
            };

            var response = await _s3Client.ListObjectsV2Async(request, ct);

            foreach (var obj in response.S3Objects)
            {
                keys.Add(obj.Key);
            }

            continuationToken = response.IsTruncated == true ? response.NextContinuationToken : null;
        } while (continuationToken != null);

        return keys;
    }

    private class UntrustedCertClientFactory : HttpClientFactory
    {
        public override HttpClient CreateHttpClient(IClientConfig clientConfig)
        {
            var handler = new HttpClientHandler
            {
                ServerCertificateCustomValidationCallback = (message, cert, chain, errors) => true
            };
            return new HttpClient(handler);
        }
    }
}
