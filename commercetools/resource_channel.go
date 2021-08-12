package commercetools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/labd/commercetools-go-sdk/commercetools"
)

func resourceChannel() *schema.Resource {
	return &schema.Resource{
		Description: "Channels represent a source or destination of different entities. They can be used to model " +
			"warehouses or stores.\n\n" +
			"See also the [Channels API Documentation](https://docs.commercetools.com/api/projects/channels)",
		Create: resourceChannelCreate,
		Read:   resourceChannelRead,
		Update: resourceChannelUpdate,
		Delete: resourceChannelDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
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

func resourceChannelCreate(d *schema.ResourceData, m interface{}) error {
	name := commercetools.LocalizedString(
		expandStringMap(d.Get("name").(map[string]interface{})))
	description := commercetools.LocalizedString(
		expandStringMap(d.Get("description").(map[string]interface{})))

	roles := []commercetools.ChannelRoleEnum{}
	for _, value := range expandStringArray(d.Get("roles").([]interface{})) {
		roles = append(roles, commercetools.ChannelRoleEnum(value))
	}

	draft := &commercetools.ChannelDraft{
		Key:         d.Get("key").(string),
		Roles:       roles,
		Name:        &name,
		Description: &description,
	}

	//custom fields are set to be filled
	if d.HasChange("custom") {
		typeId, fields := getCustomFieldsData(d)

		draft.Custom = &commercetools.CustomFieldsDraft{
			Type:   typeId,
			Fields: fields,
		}
	}

	client := getClient(m)
	var channel *commercetools.Channel

	err := resource.Retry(20*time.Second, func() *resource.RetryError {
		var err error

		channel, err = client.ChannelCreate(context.Background(), draft)
		if err != nil {
			return handleCommercetoolsError(err)
		}
		return nil
	})

	if err != nil {
		return err
	}

	d.SetId(channel.ID)
	if err := d.Set("version", channel.Version); err != nil {
		return fmt.Errorf("error reading channel: %s", err)
	}
	return resourceChannelRead(d, m)
}

func getCustomFieldsData(d *schema.ResourceData) (*commercetools.TypeResourceIdentifier, *commercetools.FieldContainer) {
	custom := d.Get("custom").([]interface{})[0].(map[string]interface{})

	typeId := &commercetools.TypeResourceIdentifier{
		Key: custom["type_key"].(string),
	}

	fields := &commercetools.FieldContainer{}

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

func resourceChannelRead(d *schema.ResourceData, m interface{}) error {
	client := getClient(m)
	channel, err := client.ChannelGetWithID(context.Background(), d.Id(), commercetools.WithReferenceExpansion("custom.type"))

	if err != nil {
		if ctErr, ok := err.(commercetools.ErrorResponse); ok {
			if ctErr.StatusCode == 404 {
				d.SetId("")
				return nil
			}
		}
		return err
	}

	d.SetId(channel.ID)
	if err := d.Set("version", channel.Version); err != nil {
		return fmt.Errorf("error reading channel: %s", err)
	}

	if channel.Name != nil {
		if err := d.Set("name", *channel.Name); err != nil {
			return fmt.Errorf("error reading channel: %s", err)
		}
	}
	if channel.Description != nil {
		if err := d.Set("description", *channel.Description); err != nil {
			return fmt.Errorf("error reading channel: %s", err)
		}
	}
	if err := d.Set("roles", channel.Roles); err != nil {
		return fmt.Errorf("error reading channel: %s", err)
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
			return fmt.Errorf("error reading channel: %s", err)
		}
	}
	return nil
}

func resourceChannelUpdate(d *schema.ResourceData, m interface{}) error {
	client := getClient(m)

	input := &commercetools.ChannelUpdateWithIDInput{
		ID:      d.Id(),
		Version: d.Get("version").(int),
		Actions: []commercetools.ChannelUpdateAction{},
	}

	if d.HasChange("key") {
		newKey := d.Get("key").(string)
		input.Actions = append(
			input.Actions,
			&commercetools.ChannelChangeKeyAction{Key: newKey})
	}

	if d.HasChange("name") {
		newName := commercetools.LocalizedString(
			expandStringMap(d.Get("name").(map[string]interface{})))
		input.Actions = append(
			input.Actions,
			&commercetools.ChannelChangeNameAction{Name: &newName})
	}

	if d.HasChange("description") {
		newDescription := commercetools.LocalizedString(
			expandStringMap(d.Get("description").(map[string]interface{})))
		input.Actions = append(
			input.Actions,
			&commercetools.ChannelChangeDescriptionAction{Description: &newDescription})
	}

	if d.HasChange("roles") {
		roles := []commercetools.ChannelRoleEnum{}
		for _, value := range expandStringArray(d.Get("roles").([]interface{})) {
			roles = append(roles, commercetools.ChannelRoleEnum(value))
		}
		input.Actions = append(
			input.Actions,
			&commercetools.ChannelSetRolesAction{Roles: roles})
	}

	if d.HasChange("custom") {
		typeId, fields := getCustomFieldsData(d)

		input.Actions = append(
			input.Actions,
			&commercetools.ChannelSetCustomTypeAction{Type: typeId, Fields: fields})
	}

	_, err := client.ChannelUpdateWithID(context.Background(), input)
	if err != nil {
		return err
	}

	return resourceChannelRead(d, m)
}

func resourceChannelDelete(d *schema.ResourceData, m interface{}) error {
	client := getClient(m)
	version := d.Get("version").(int)
	_, err := client.ChannelDeleteWithID(context.Background(), d.Id(), version)
	if err != nil {
		return err
	}

	return nil
}
