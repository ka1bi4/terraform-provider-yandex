//revive:disable:var-naming
package yandex

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/yandex-cloud/go-genproto/yandex/cloud/iam/v1/awscompatibility"
)

func init() {
	resource.AddTestSweepers("yandex_storage_object", &resource.Sweeper{
		Name:         "yandex_storage_object",
		F:            testSweepStorageObject,
		Dependencies: []string{},
	})
}

func testSweepStorageObject(_ string) error {
	conf, err := configForSweepers()
	if err != nil {
		return fmt.Errorf("error getting client: %s", err)
	}

	result := &multierror.Error{}
	serviceAccountID, err := createIAMServiceAccountForSweeper(conf)
	if serviceAccountID != "" {
		defer func() {
			if !sweepIAMServiceAccount(conf, serviceAccountID) {
				result = multierror.Append(result,
					fmt.Errorf("failed to sweep IAM service account %q", serviceAccountID))
			}
		}()
	}
	if err != nil {
		result = multierror.Append(result, fmt.Errorf("error creating service account: %s", err))
		return result.ErrorOrNil()
	}

	resp, err := conf.sdk.IAM().AWSCompatibility().AccessKey().Create(conf.Context(), &awscompatibility.CreateAccessKeyRequest{
		ServiceAccountId: serviceAccountID,
		Description:      "Storage Bucket sweeper static key",
	})
	if err != nil {
		result = multierror.Append(result, fmt.Errorf("error creating service account static key: %s", err))
		return result.ErrorOrNil()
	}

	defer func() {
		_, err := conf.sdk.IAM().AWSCompatibility().AccessKey().Delete(conf.Context(), &awscompatibility.DeleteAccessKeyRequest{
			AccessKeyId: resp.AccessKey.Id,
		})
		if err != nil {
			result = multierror.Append(result, fmt.Errorf("error deleting service account static key: %s", err))
		}
	}()

	s3client, err := getS3ClientByKeys(resp.AccessKey.KeyId, resp.Secret, conf)
	if err != nil {
		result = multierror.Append(result, fmt.Errorf("error creating storage client: %s", err))
		return result.ErrorOrNil()
	}

	buckets, err := s3client.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		result = multierror.Append(result, fmt.Errorf("failed to list storage buckets: %s", err))
		return result.ErrorOrNil()
	}

	for _, b := range buckets.Buckets {
		res, err := s3client.ListObjects(&s3.ListObjectsInput{
			Bucket: b.Name,
		})

		if err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to list objects in bucket: %s, error: %s", *b.Name, err))
		}

		for _, o := range res.Contents {
			_, err := s3client.DeleteObject(&s3.DeleteObjectInput{
				Bucket: b.Name,
				Key:    o.Key,
			})
			if err != nil {
				result = multierror.Append(result, fmt.Errorf("failed to delete object %s in bucket: %s, error: %s", *o.Key, *b.Name, err))
			}
		}
	}

	return result.ErrorOrNil()
}

func TestAccStorageObject_source(t *testing.T) {
	var obj s3.GetObjectOutput
	resourceName := "yandex_storage_object.test"
	rInt := acctest.RandInt()

	source := testAccStorageObjectCreateTempFile(t, "some_bucket_content")
	defer os.Remove(source)

	resource.Test(t, resource.TestCase{
		PreCheck:        func() { testAccPreCheck(t) },
		IDRefreshName:   resourceName,
		IDRefreshIgnore: []string{"access_key", "secret_key"},
		Providers:       testAccProviders,
		CheckDestroy:    testAccCheckStorageObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccStorageObjectConfigSource(rInt, source),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageObjectExists(resourceName, &obj),
					testAccCheckStorageObjectBody(&obj, "some_bucket_content"),
				),
			},
		},
	})
}

func TestAccStorageObject_content(t *testing.T) {
	var obj s3.GetObjectOutput
	resourceName := "yandex_storage_object.test"
	rInt := acctest.RandInt()

	resource.Test(t, resource.TestCase{
		PreCheck:        func() { testAccPreCheck(t) },
		IDRefreshName:   resourceName,
		IDRefreshIgnore: []string{"access_key", "secret_key"},
		Providers:       testAccProviders,
		CheckDestroy:    testAccCheckStorageObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccStorageObjectConfigContent(rInt, "some_bucket_content"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageObjectExists(resourceName, &obj),
					testAccCheckStorageObjectBody(&obj, "some_bucket_content"),
				),
			},
		},
	})
}

func TestAccStorageObject_contentBase64(t *testing.T) {
	var obj s3.GetObjectOutput
	resourceName := "yandex_storage_object.test"
	rInt := acctest.RandInt()

	resource.Test(t, resource.TestCase{
		PreCheck:        func() { testAccPreCheck(t) },
		IDRefreshName:   resourceName,
		IDRefreshIgnore: []string{"access_key", "secret_key"},
		Providers:       testAccProviders,
		CheckDestroy:    testAccCheckStorageObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccStorageObjectConfigContentBase64(rInt, base64.StdEncoding.EncodeToString([]byte("some_bucket_content"))),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageObjectExists(resourceName, &obj),
					testAccCheckStorageObjectBody(&obj, "some_bucket_content"),
				),
			},
		},
	})
}
func TestAccStorageObject_contentTypeEmpty(t *testing.T) {
	var obj s3.GetObjectOutput
	resourceName := "yandex_storage_object.test"
	rInt := acctest.RandInt()

	resource.Test(t, resource.TestCase{
		PreCheck:        func() { testAccPreCheck(t) },
		IDRefreshName:   resourceName,
		IDRefreshIgnore: []string{"access_key", "secret_key"},
		Providers:       testAccProviders,
		CheckDestroy:    testAccCheckStorageObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccStorageObjectConfigContentType(rInt, ""),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageObjectExists(resourceName, &obj),
					testAccCheckStorageObjectContentType(&obj, "application/octet-stream"),
				),
			},
		},
	})
}
func TestAccStorageObject_contentTypeText(t *testing.T) {
	var obj s3.GetObjectOutput
	resourceName := "yandex_storage_object.test"
	rInt := acctest.RandInt()

	resource.Test(t, resource.TestCase{
		PreCheck:        func() { testAccPreCheck(t) },
		IDRefreshName:   resourceName,
		IDRefreshIgnore: []string{"access_key", "secret_key"},
		Providers:       testAccProviders,
		CheckDestroy:    testAccCheckStorageObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccStorageObjectConfigContentType(rInt, "text/plain"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageObjectExists(resourceName, &obj),
					testAccCheckStorageObjectContentType(&obj, "text/plain"),
				),
			},
		},
	})
}

func TestAccStorageObject_updateAcl(t *testing.T) {
	var obj s3.GetObjectOutput
	rInt := acctest.RandInt()
	resourceName := "yandex_storage_object.test"

	resource.Test(t, resource.TestCase{
		PreCheck:        func() { testAccPreCheck(t) },
		IDRefreshName:   resourceName,
		IDRefreshIgnore: []string{"access_key", "secret_key"},
		Providers:       testAccProviders,
		CheckDestroy:    testAccCheckStorageObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccStorageObjectAclPreConfig(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageObjectExists(resourceName, &obj),
					resource.TestCheckResourceAttr(resourceName, "acl", "public-read"),
				),
			},
			{
				Config: testAccStorageObjectAclPostConfig(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageObjectExists(resourceName, &obj),
					resource.TestCheckResourceAttr(resourceName, "acl", "private"),
				),
			},
		},
	})
}

func testAccCheckStorageObjectDestroy(s *terraform.State) error {
	return testAccCheckStorageObjectDestroyWithProvider(s, testAccProvider)
}

func testAccCheckStorageObjectDestroyWithProvider(s *terraform.State, provider *schema.Provider) error {
	config := provider.Meta().(*Config)

	// access and secret keys should be destroyed too and defaults may be not provided, so create temporary ones
	ak, sak, cleanup, err := createTemporaryStaticAccessKey("editor", config)
	if err != nil {
		return err
	}
	defer cleanup()

	s3conn, err := getS3ClientByKeys(ak, sak, config)
	if err != nil {
		return err
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "yandex_storage_object" {
			continue
		}

		_, err := s3conn.HeadObject(
			&s3.HeadObjectInput{
				Bucket: aws.String(rs.Primary.Attributes["bucket"]),
				Key:    aws.String(rs.Primary.Attributes["key"]),
			})
		if err == nil {
			return fmt.Errorf("storage object still exists: %s", rs.Primary.ID)
		}
	}
	return nil
}

func testAccCheckStorageObjectExists(n string, obj *s3.GetObjectOutput) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no storage object ID is set")
		}

		s3conn, err := getS3ClientByKeys(rs.Primary.Attributes["access_key"], rs.Primary.Attributes["secret_key"],
			testAccProvider.Meta().(*Config))
		if err != nil {
			return err
		}

		out, err := s3conn.GetObject(
			&s3.GetObjectInput{
				Bucket: aws.String(rs.Primary.Attributes["bucket"]),
				Key:    aws.String(rs.Primary.Attributes["key"]),
			})
		if err != nil {
			return fmt.Errorf("storage object error: %s", err)
		}

		*obj = *out

		return nil
	}
}

func testAccCheckStorageObjectBody(obj *s3.GetObjectOutput, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		body, err := ioutil.ReadAll(obj.Body)
		if err != nil {
			return fmt.Errorf("failed to read body: %s", err)
		}
		obj.Body.Close()

		if got := string(body); got != want {
			return fmt.Errorf("wrong result body %q; want %q", got, want)
		}

		return nil
	}
}
func testAccCheckStorageObjectContentType(obj *s3.GetObjectOutput, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if got := *obj.ContentType; got != want {
			return fmt.Errorf("wrong result content_type %q; want %q", got, want)
		}

		return nil
	}
}

func testAccStorageObjectCreateTempFile(t *testing.T, data string) string {
	tmpFile, err := ioutil.TempFile("", "tf-acc-storage-obj")
	if err != nil {
		t.Fatal(err)
	}
	filename := tmpFile.Name()

	err = ioutil.WriteFile(filename, []byte(data), 0644)
	if err != nil {
		os.Remove(filename)
		t.Fatal(err)
	}

	return filename
}

func testAccStorageObjectConfigSource(randInt int, source string) string {
	return fmt.Sprintf(`
resource "yandex_storage_bucket" "test" {
	bucket = "tf-object-test-bucket-%[1]d"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key
}

resource "yandex_storage_object" "test" {
	bucket = "${yandex_storage_bucket.test.bucket}"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key

	key     = "test-key"
	source  = "%[2]s"
}
`, randInt, source) + testAccCommonIamDependenciesEditorConfig(randInt)
}

func testAccStorageObjectConfigContent(randInt int, content string) string {
	return fmt.Sprintf(`
resource "yandex_storage_bucket" "test" {
	bucket = "tf-object-test-bucket-%[1]d"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key
}

resource "yandex_storage_object" "test" {
	bucket = "${yandex_storage_bucket.test.bucket}"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key

	key     = "test-key"
	content = "%[2]s"
}
`, randInt, content) + testAccCommonIamDependenciesEditorConfig(randInt)
}

func testAccStorageObjectConfigContentBase64(randInt int, contentBase64 string) string {
	return fmt.Sprintf(`
resource "yandex_storage_bucket" "test" {
	bucket = "tf-object-test-bucket-%[1]d"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key
}

resource "yandex_storage_object" "test" {
	bucket = "${yandex_storage_bucket.test.bucket}"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key

	key            = "test-key"
	content_base64 = "%[2]s"
}
`, randInt, contentBase64) + testAccCommonIamDependenciesEditorConfig(randInt)
}

func testAccStorageObjectConfigContentType(randInt int, contentType string) string {
	return fmt.Sprintf(`
resource "yandex_storage_bucket" "test" {
	bucket = "tf-object-test-bucket-%[1]d"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key
}

resource "yandex_storage_object" "test" {
	bucket = "${yandex_storage_bucket.test.bucket}"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key

	key            = "test-key"
	content        = "some-content-type"
	content_type   = "%[2]s"
}
`, randInt, contentType) + testAccCommonIamDependenciesEditorConfig(randInt)
}

func testAccStorageObjectAclPreConfig(randInt int) string {
	return fmt.Sprintf(`
resource "yandex_storage_bucket" "test" {
	bucket = "tf-object-test-bucket-%[1]d"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key
}

resource "yandex_storage_object" "test" {
	bucket = "${yandex_storage_bucket.test.bucket}"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key

	key     = "test-key"
	content = "some-contect"

	acl = "public-read"
}
`, randInt) + testAccCommonIamDependenciesAdminConfig(randInt)
}

func testAccStorageObjectAclPostConfig(randInt int) string {
	return fmt.Sprintf(`
resource "yandex_storage_bucket" "test" {
	bucket = "tf-object-test-bucket-%[1]d"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key
}

resource "yandex_storage_object" "test" {
	bucket = "${yandex_storage_bucket.test.bucket}"

	access_key = yandex_iam_service_account_static_access_key.sa-key.access_key
	secret_key = yandex_iam_service_account_static_access_key.sa-key.secret_key

	key     = "test-key"
	content = "some-contect"

	acl = "private"
}
`, randInt) + testAccCommonIamDependenciesAdminConfig(randInt)
}
