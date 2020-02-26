package v7

import (
	"fmt"
	"sort"
	"strings"

	"code.cloudfoundry.org/cli/actor/sharedaction"
	"code.cloudfoundry.org/cli/actor/v7action"
	"code.cloudfoundry.org/cli/command"
	"code.cloudfoundry.org/cli/command/flag"
	"code.cloudfoundry.org/cli/command/translatableerror"
	"code.cloudfoundry.org/cli/command/v7/shared"
	"code.cloudfoundry.org/cli/types"
	"code.cloudfoundry.org/cli/util/ui"
	"code.cloudfoundry.org/clock"
)

type ResourceType string

const (
	App             ResourceType = "app"
	Buildpack       ResourceType = "buildpack"
	Domain          ResourceType = "domain"
	Org             ResourceType = "org"
	Route           ResourceType = "route"
	Space           ResourceType = "space"
	Stack           ResourceType = "stack"
	ServiceBroker   ResourceType = "service-broker"
	ServiceOffering ResourceType = "service-offering"
	ServicePlan     ResourceType = "service-plan"
)

//go:generate counterfeiter . LabelsActor

type LabelsActor interface {
	GetApplicationLabels(appName string, spaceGUID string) (map[string]types.NullString, v7action.Warnings, error)
	GetDomainLabels(domainName string) (map[string]types.NullString, v7action.Warnings, error)
	GetOrganizationLabels(orgName string) (map[string]types.NullString, v7action.Warnings, error)
	GetRouteLabels(routeName string, spaceGUID string) (map[string]types.NullString, v7action.Warnings, error)
	GetSpaceLabels(spaceName string, orgGUID string) (map[string]types.NullString, v7action.Warnings, error)
	GetBuildpackLabels(buildpackName string, buildpackStack string) (map[string]types.NullString, v7action.Warnings, error)
	GetStackLabels(stackName string) (map[string]types.NullString, v7action.Warnings, error)
	GetServiceBrokerLabels(serviceBrokerName string) (map[string]types.NullString, v7action.Warnings, error)
	GetServiceOfferingLabels(serviceOfferingName, serviceBrokerName string) (map[string]types.NullString, v7action.Warnings, error)
	GetServicePlanLabels(servicePlanName, serviceOfferingName, serviceBrokerName string) (map[string]types.NullString, v7action.Warnings, error)
}

type LabelsCommand struct {
	RequiredArgs    flag.LabelsArgs `positional-args:"yes"`
	BuildpackStack  string          `long:"stack" short:"s" description:"Specify stack to disambiguate buildpacks with the same name"`
	usage           interface{}     `usage:"CF_NAME labels RESOURCE RESOURCE_NAME\n\nEXAMPLES:\n   cf labels app dora\n   cf labels org business\n   cf labels buildpack go_buildpack --stack cflinuxfs3 \n\nRESOURCES:\n   app\n   buildpack\n   domain\n   org\n   route\n   service-broker\n   service-offering\n   service-plan\n   space\n   stack"`
	relatedCommands interface{}     `related_commands:"set-label, unset-label"`
	ServiceBroker   string          `long:"broker" short:"b" description:"Specify a service broker to disambiguate service offerings or service plans with the same name."`
	ServiceOffering string          `long:"offering" short:"e" description:"Specify a service offering to disambiguate service plans with the same name."`

	UI          command.UI
	Config      command.Config
	SharedActor command.SharedActor
	Actor       LabelsActor

	username string
}

func (cmd *LabelsCommand) Setup(config command.Config, ui command.UI) error {
	cmd.UI = ui
	cmd.Config = config
	cmd.SharedActor = sharedaction.NewActor(config)
	ccClient, _, err := shared.GetNewClientsAndConnectToCF(config, ui, "")
	if err != nil {
		return err
	}
	cmd.Actor = v7action.NewActor(ccClient, config, nil, nil, clock.NewClock())
	return nil
}

func (cmd LabelsCommand) Execute(args []string) error {
	var (
		labels   map[string]types.NullString
		warnings v7action.Warnings
		err      error
	)

	cmd.username, err = cmd.Config.CurrentUserName()
	if err != nil {
		return err
	}

	if err := cmd.validateFlags(); err != nil {
		return err
	}

	if err := cmd.checkTarget(); err != nil {
		return err
	}

	switch cmd.canonicalResourceTypeForName() {
	case App:
		cmd.displayMessageWithOrgAndSpace()
		labels, warnings, err = cmd.Actor.GetApplicationLabels(cmd.RequiredArgs.ResourceName, cmd.Config.TargetedSpace().GUID)
	case Buildpack:
		cmd.displayMessageWithStack()
		labels, warnings, err = cmd.Actor.GetBuildpackLabels(cmd.RequiredArgs.ResourceName, cmd.BuildpackStack)
	case Domain:
		cmd.displayMessageDefault()
		labels, warnings, err = cmd.Actor.GetDomainLabels(cmd.RequiredArgs.ResourceName)
	case Org:
		cmd.displayMessageDefault()
		labels, warnings, err = cmd.Actor.GetOrganizationLabels(cmd.RequiredArgs.ResourceName)
	case Route:
		cmd.displayMessageWithOrgAndSpace()
		labels, warnings, err = cmd.Actor.GetRouteLabels(cmd.RequiredArgs.ResourceName, cmd.Config.TargetedSpace().GUID)
	case ServiceBroker:
		cmd.displayMessageDefault()
		labels, warnings, err = cmd.Actor.GetServiceBrokerLabels(cmd.RequiredArgs.ResourceName)
	case ServiceOffering:
		cmd.displayMessageForServiceCommands()
		labels, warnings, err = cmd.Actor.GetServiceOfferingLabels(cmd.RequiredArgs.ResourceName, cmd.ServiceBroker)
	case ServicePlan:
		cmd.displayMessageForServiceCommands()
		labels, warnings, err = cmd.Actor.GetServicePlanLabels(cmd.RequiredArgs.ResourceName, cmd.ServiceOffering, cmd.ServiceBroker)
	case Space:
		cmd.displayMessageWithOrg()
		labels, warnings, err = cmd.Actor.GetSpaceLabels(cmd.RequiredArgs.ResourceName, cmd.Config.TargetedOrganization().GUID)
	case Stack:
		cmd.displayMessageDefault()
		labels, warnings, err = cmd.Actor.GetStackLabels(cmd.RequiredArgs.ResourceName)
	default:
		err = fmt.Errorf("Unsupported resource type of '%s'", cmd.RequiredArgs.ResourceType)
	}
	cmd.UI.DisplayWarnings(warnings)
	if err != nil {
		return err
	}

	cmd.printLabels(labels)
	return nil
}

func (cmd LabelsCommand) canonicalResourceTypeForName() ResourceType {
	return ResourceType(strings.ToLower(cmd.RequiredArgs.ResourceType))
}

func (cmd LabelsCommand) printLabels(labels map[string]types.NullString) {
	if len(labels) == 0 {
		cmd.UI.DisplayText("No labels found.")
		return
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	table := [][]string{
		{
			cmd.UI.TranslateText("key"),
			cmd.UI.TranslateText("value"),
		},
	}

	for _, key := range keys {
		table = append(table, []string{key, labels[key].Value})
	}

	cmd.UI.DisplayTableWithHeader("", table, ui.DefaultTableSpacePadding)
}

func (cmd LabelsCommand) validateFlags() error {
	resourceType := cmd.canonicalResourceTypeForName()
	if cmd.BuildpackStack != "" && resourceType != Buildpack {
		return translatableerror.ArgumentCombinationError{
			Args: []string{
				cmd.RequiredArgs.ResourceType, "--stack, -s",
			},
		}
	}

	if cmd.ServiceBroker != "" && !(resourceType == ServiceOffering || resourceType == ServicePlan) {
		return translatableerror.ArgumentCombinationError{
			Args: []string{
				cmd.RequiredArgs.ResourceType, "--broker, -b",
			},
		}
	}

	if cmd.ServiceOffering != "" && resourceType != ServicePlan {
		return translatableerror.ArgumentCombinationError{
			Args: []string{
				cmd.RequiredArgs.ResourceType, "--offering, -o",
			},
		}
	}

	return nil
}

func (cmd LabelsCommand) checkTarget() error {
	switch ResourceType(cmd.RequiredArgs.ResourceType) {
	case App, Route:
		return cmd.SharedActor.CheckTarget(true, true)
	case Space:
		return cmd.SharedActor.CheckTarget(true, false)
	default:
		return cmd.SharedActor.CheckTarget(false, false)
	}
}

func (cmd LabelsCommand) displayMessageDefault() {
	cmd.UI.DisplayTextWithFlavor("Getting labels for {{.ResourceType}} {{.ResourceName}} as {{.User}}...", map[string]interface{}{
		"ResourceType": cmd.RequiredArgs.ResourceType,
		"ResourceName": cmd.RequiredArgs.ResourceName,
		"User":         cmd.username,
	})

	cmd.UI.DisplayNewline()
}

func (cmd LabelsCommand) displayMessageWithOrgAndSpace() {
	cmd.UI.DisplayTextWithFlavor("Getting labels for {{.ResourceType}} {{.ResourceName}} in org {{.OrgName}} / space {{.SpaceName}} as {{.User}}...", map[string]interface{}{
		"ResourceType": cmd.RequiredArgs.ResourceType,
		"ResourceName": cmd.RequiredArgs.ResourceName,
		"OrgName":      cmd.Config.TargetedOrganization().Name,
		"SpaceName":    cmd.Config.TargetedSpace().Name,
		"User":         cmd.username,
	})

	cmd.UI.DisplayNewline()
}

func (cmd LabelsCommand) displayMessageWithOrg() {
	cmd.UI.DisplayTextWithFlavor("Getting labels for {{.ResourceType}} {{.ResourceName}} in org {{.OrgName}} as {{.User}}...", map[string]interface{}{
		"ResourceType": cmd.RequiredArgs.ResourceType,
		"ResourceName": cmd.RequiredArgs.ResourceName,
		"OrgName":      cmd.Config.TargetedOrganization().Name,
		"User":         cmd.username,
	})

	cmd.UI.DisplayNewline()
}

func (cmd LabelsCommand) displayMessageWithStack() {
	var template string
	if cmd.BuildpackStack == "" {
		template = "Getting labels for {{.ResourceType}} {{.ResourceName}} as {{.User}}..."
	} else {
		template = "Getting labels for {{.ResourceType}} {{.ResourceName}} with stack {{.StackName}} as {{.User}}..."
	}

	cmd.UI.DisplayTextWithFlavor(template, map[string]interface{}{
		"ResourceType": cmd.RequiredArgs.ResourceType,
		"ResourceName": cmd.RequiredArgs.ResourceName,
		"StackName":    cmd.BuildpackStack,
		"User":         cmd.username,
	})

	cmd.UI.DisplayNewline()
}

func (cmd LabelsCommand) displayMessageForServiceCommands() {
	var template string
	template = "Getting labels for {{.ResourceType}} {{.ResourceName}}"

	if cmd.ServiceOffering != "" || cmd.ServiceBroker != "" {
		template += " from"
	}
	if cmd.ServiceOffering != "" {
		template += " service offering {{.ServiceOffering}}"
		if cmd.ServiceBroker != "" {
			template += " /"
		}
	}

	if cmd.ServiceBroker != "" {
		template += " service broker {{.ServiceBroker}}"
	}

	template += " as {{.User}}..."

	cmd.UI.DisplayTextWithFlavor(template, map[string]interface{}{
		"ResourceName":    cmd.RequiredArgs.ResourceName,
		"ResourceType":    cmd.RequiredArgs.ResourceType,
		"ServiceBroker":   cmd.ServiceBroker,
		"ServiceOffering": cmd.ServiceOffering,
		"User":            cmd.username,
	})

	cmd.UI.DisplayNewline()
}
