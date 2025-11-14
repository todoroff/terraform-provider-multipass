package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

func TestFilterImages(t *testing.T) {
	images := []models.Image{
		{Name: "24.04", Aliases: []string{"noble", "lts"}, Kind: models.ImageKindImage, Description: "Ubuntu 24.04"},
		{Name: "docker", Aliases: []string{}, Kind: models.ImageKindBlueprint, Description: "Docker env"},
	}

	cfg := imagesDataSourceModel{
		Alias: types.StringValue("lts"),
	}

	got := filterImages(images, cfg)
	if len(got) != 1 || got[0].Name != "24.04" {
		t.Fatalf("expected lts image, got %#v", got)
	}
}
