package commercetools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/labd/commercetools-go-sdk/platform"
)

func resourceChannel() *schema.Resource {
	return &schema.Resource{
		Description: "Channels represent a source or destination of different entities. They can be used to model " +
			"warehouses or stores.\n\n" +
			"See also the [Channels API Documentation](https://docs.commercetools.com/api/projects/channels)",
		CreateContext: resourceChannelCreate,
		ReadContext:   resourceChannelRead,
		UpdateContext: resourceChannelUpdate,
		DeleteContext: resourceChannelDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"key": {
				Description: "Any arbitrary string key that uniquely identifies this channel within the project",
				Type:        schema.TypeString,
				Required:    true,
			},
			"roles": {
				Description: "The [roles](https://docs.commercetools.com/api/projects/channels#channelroleenum) " +
					"of this channel. Each channel must have at least one role",
				Type:     schema.TypeList,
				Required: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"name": {
				Description: "[LocalizedString](https://docs.commercetools.com/api/types#localizedstring)",
				Type:        TypeLocalizedString,
				Optional:    true,
			},
			"description": {
				Description: "[LocalizedString](https://docs.commercetools.com/api/types#localizedstring)",
				Type:        TypeLocalizedString,
				Optional:    true,
			},
			"version": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"custom": {
				Type:     schema.TypeList,
				MaxItems: 1,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type_key": {
							Type:     schema.TypeString,
							Required: true,
						},
						"field": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"key": {
										Type:     schema.TypeString,
										Required: true,
									},
									"value": {
										Type:        schema.TypeString,
										Optional:    true,
										Description: "The value of a custom field (https://docs.commercetools.com/api/projects/channels#set-customfield)",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func resourceChannelCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	name := unmarshallLocalizedString(d.Get("name"))
	description := unmarshallLocalizedString(d.Get("description"))

	roles := []platform.ChannelRoleEnum{}
	for _, value := range expandStringArray(d.Get("roles").([]interface{})) {
		roles = append(roles, platform.ChannelRoleEnum(value))
	}

	draft := platform.ChannelDraft{
		Key:         d.Get("key").(string),
		Roles:       roles,
		Name:        &name,
		Description: &description,
	}

	//custom fields are set to be filled
	if d.HasChange("custom") {
		typeId, fields := getCustomFieldsData(d)

		draft.Custom = &platform.CustomFieldsDraft{
			Type:   *typeId,
			Fields: fields,
		}
	}

	client := getClient(m)
	var channel *platform.Channel

	err := resource.RetryContext(ctx, 20*time.Second, func() *resource.RetryError {
		var err error

		channel, err = client.Channels().Post(draft).Execute(ctx)
		if err != nil {
			return handleCommercetoolsError(err)
		}
		return nil
	})

	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(channel.ID)
	if err := d.Set("version", channel.Version); err != nil {
		return diag.FromErr(err)
	}
	return resourceChannelRead(ctx, d, m)
}

func getCustomFieldsData(d *schema.ResourceData) (*platform.TypeResourceIdentifier, *platform.FieldContainer) {
	custom := d.Get("custom").([]interface{})[0].(map[string]interface{})

	typeKey := custom["type_key"].(string)

	typeId := &platform.TypeResourceIdentifier{
		Key: &typeKey,
	}

	fields := &platform.FieldContainer{}

	for _, fieldDef := range custom["field"].([]interface{}) {
		key := fieldDef.(map[string]interface{})["key"].(string)
		value := fieldDef.(map[string]interface{})["value"].(string)
		decodedValue := _decodeCustomFieldValue(value)

		(*fields)[key] = decodedValue

	}
	return typeId, fields
}

func _decodeCustomFieldValue(value string) interface{} {
	var data interface{}
	_ = json.Unmarshal([]byte(value), &data)
	return data
}

func _encodeCustomFieldValue(value interface{}) string {
	data, _ := json.Marshal(value)

	return string(data)
}

func resourceChannelRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := getClient(m)
	channel, err := client.Channels().WithId(d.Id()).Get().Execute(ctx)

	if err != nil {
		if ctErr, ok := err.(platform.ErrorResponse); ok {
			if ctErr.StatusCode == 404 {
				d.SetId("")
				return nil
			}
		}
		return diag.FromErr(err)
	}

	d.SetId(channel.ID)
	if err := d.Set("version", channel.Version); err != nil {
		return diag.FromErr(err)
	}

	if channel.Name != nil {
		if err := d.Set("name", *channel.Name); err != nil {
			return diag.FromErr(err)
		}
	}
	if channel.Description != nil {
		if err := d.Set("description", *channel.Description); err != nil {
			return diag.FromErr(err)
		}
	}
	if err := d.Set("roles", channel.Roles); err != nil {
		return diag.FromErr(err)
	}

	if channel.Custom != nil {
		data := _decodeCustomFieldValue(_encodeCustomFieldValue(channel.Custom.Fields))

		customFields := make([]interface{}, 0)

		for fieldKey, fieldValue := range data.(map[string]interface{}) {
			customFields = append(customFields, map[string]interface{}{
				"key":   fieldKey,
				"value": _encodeCustomFieldValue(fieldValue),
			})
		}

		customBase := []interface{}{map[string]interface{}{
			"type_key": channel.Custom.Type.Obj.Key,
			"field":    customFields,
		}}

		if err := d.Set("custom", customBase); err != nil {
			return diag.FromErr(err)
		}
	}
	return nil
}

func resourceChannelUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := getClient(m)

	input := platform.ChannelUpdate{
		Version: d.Get("version").(int),
		Actions: []platform.ChannelUpdateAction{},
	}

	if d.HasChange("key") {
		newKey := d.Get("key").(string)
		input.Actions = append(
			input.Actions,
			&platform.ChannelChangeKeyAction{Key: newKey})
	}

	if d.HasChange("name") {
		newName := unmarshallLocalizedString(d.Get("name"))
		input.Actions = append(
			input.Actions,
			&platform.ChannelChangeNameAction{Name: newName})
	}

	if d.HasChange("description") {
		newDescription := unmarshallLocalizedString(d.Get("description"))
		input.Actions = append(
			input.Actions,
			&platform.ChannelChangeDescriptionAction{Description: newDescription})
	}

	if d.HasChange("roles") {
		roles := []platform.ChannelRoleEnum{}
		for _, value := range expandStringArray(d.Get("roles").([]interface{})) {
			roles = append(roles, platform.ChannelRoleEnum(value))
		}
		input.Actions = append(
			input.Actions,
			&platform.ChannelSetRolesAction{Roles: roles})
	}

	if d.HasChange("custom") {
		typeId, fields := getCustomFieldsData(d)

		input.Actions = append(
			input.Actions,
			&platform.ChannelSetCustomTypeAction{Type: typeId, Fields: fields})
	}

	_, err := client.Channels().WithId(d.Id()).Post(input).Execute(ctx)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceChannelRead(ctx, d, m)
}

func resourceChannelDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := getClient(m)
	version := d.Get("version").(int)
	_, err := client.Channels().WithId(d.Id()).Delete().Version(version).Execute(ctx)
	if err != nil {
		return diag.FromErr(err)
	}

	return nil
}
