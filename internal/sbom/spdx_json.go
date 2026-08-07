package sbom

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	spdxjson "github.com/spdx/tools-golang/json"
	"github.com/spdx/tools-golang/spdx"
	"github.com/spdx/tools-golang/spdx/v2/common"
)

type SPDXJSONFormatter struct{}

func (SPDXJSONFormatter) Format(w io.Writer, document Document) error {
	document = withDefaults(document)
	doc := toSPDXDocument(document)
	return spdxjson.Write(doc, w, spdxjson.Indent("  "))
}

func toSPDXDocument(document Document) *spdx.Document {
	doc := &spdx.Document{
		SPDXVersion:       spdx.Version,
		DataLicense:       spdx.DataLicense,
		SPDXIdentifier:    common.ElementID("DOCUMENT"),
		DocumentName:      defaultDocumentName(document.Metadata.Source),
		DocumentNamespace: namespaceFor(document),
		CreationInfo: &spdx.CreationInfo{
			Created: document.Metadata.GeneratedAt.Format("2006-01-02T15:04:05Z"),
			Creators: []common.Creator{
				{
					CreatorType: "Tool",
					Creator:     fmt.Sprintf("%s-%s", document.Metadata.Tool.Name, document.Metadata.Tool.Version),
				},
			},
			CreatorComment: sourceComment(document.Metadata.Source),
		},
	}

	documentRef := common.DocElementID{ElementRefID: doc.SPDXIdentifier}
	for idx, component := range document.Components {
		packageID := common.ElementID(fmt.Sprintf("Package-%03d", idx+1))
		pkg := &spdx.Package{
			PackageName:               component.Repository,
			PackageSPDXIdentifier:     packageID,
			PackageVersion:            componentVersion(component),
			PackageDownloadLocation:   "NOASSERTION",
			FilesAnalyzed:             false,
			IsFilesAnalyzedTagPresent: true,
			PrimaryPackagePurpose:     "CONTAINER",
			PackageExternalReferences: []*spdx.PackageExternalReference{
				{
					Category: common.CategoryPackageManager,
					RefType:  common.TypePackageManagerPURL,
					Locator:  packageURL(component),
				},
			},
			PackageSummary:          component.Reference,
			PackageLicenseDeclared:  "NOASSERTION",
			PackageLicenseConcluded: "NOASSERTION",
			PackageCopyrightText:    "NOASSERTION",
		}

		if len(component.Evidence) != 0 {
			pkg.Annotations = append(pkg.Annotations, spdx.Annotation{
				AnnotationDate: document.Metadata.GeneratedAt.Format("2006-01-02T15:04:05Z"),
				AnnotationType: "OTHER",
				Annotator: common.Annotator{
					AnnotatorType: "Tool",
					Annotator:     fmt.Sprintf("%s-%s", document.Metadata.Tool.Name, document.Metadata.Tool.Version),
				},
				AnnotationComment: "Manifest evidence: " + strings.Join(component.Evidence, ", "),
			})
		}

		doc.Packages = append(doc.Packages, pkg)
		doc.Relationships = append(doc.Relationships, &spdx.Relationship{
			RefA:         documentRef,
			RefB:         common.DocElementID{ElementRefID: packageID},
			Relationship: common.TypeRelationshipDescribe,
		})
	}

	return doc
}

func namespaceFor(document Document) string {
	hash := sha1.Sum([]byte(strings.Join([]string{
		document.Metadata.Source.Chart,
		document.Metadata.Source.ReleaseName,
		document.Metadata.Source.Namespace,
		document.Metadata.GeneratedAt.Format("2006-01-02T15:04:05Z"),
	}, "|")))

	return "https://codesphere-cloud.github.io/bom/spdx/" + hex.EncodeToString(hash[:])
}

func sourceComment(source SourceMetadata) string {
	parts := []string{
		fmt.Sprintf("chart=%s", source.Chart),
		fmt.Sprintf("chartName=%s", source.ChartName),
		fmt.Sprintf("releaseName=%s", source.ReleaseName),
		fmt.Sprintf("namespace=%s", source.Namespace),
	}

	if len(source.ValuesFiles) != 0 {
		parts = append(parts, "values="+strings.Join(source.ValuesFiles, ","))
	}
	if len(source.SetValues) != 0 {
		parts = append(parts, "set="+strings.Join(source.SetValues, ","))
	}
	if len(source.SetStrings) != 0 {
		parts = append(parts, "set-string="+strings.Join(source.SetStrings, ","))
	}
	if len(source.HelmArgs) != 0 {
		parts = append(parts, "helm-arg="+strings.Join(source.HelmArgs, ","))
	}

	return "Rendered from Helm inputs: " + strings.Join(parts, "; ")
}

func defaultDocumentName(source SourceMetadata) string {
	if source.ChartName != "" {
		return source.ChartName
	}
	return source.ReleaseName
}

func componentVersion(component Component) string {
	if component.Tag != "" {
		return component.Tag
	}
	return component.Digest
}

func packageURL(component Component) string {
	ref := component.Tag
	if ref == "" {
		ref = component.Digest
	}
	if ref == "" {
		return "pkg:oci/" + component.Repository
	}

	return "pkg:oci/" + component.Repository + "@" + ref
}
