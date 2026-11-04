package accountapi

import (
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/workflowauth"
)

func workflowProfile(doc profiledocument.Document) (map[string]any, map[string]any) {
	return workflowauth.CareerProjection(doc)
}
