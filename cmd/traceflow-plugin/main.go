package main

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/goccy/go-graphviz"
	"github.com/vmware/octant/pkg/icon"
	"github.com/vmware/octant/pkg/navigation"
	"github.com/vmware/octant/pkg/plugin"
	"github.com/vmware/octant/pkg/plugin/service"
	"github.com/vmware/octant/pkg/view/component"
	"github.com/vmware/octant/pkg/view/flexlayout"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/vmware-tanzu/antrea/pkg/apis/traceflow/v1"
	clientset "github.com/vmware-tanzu/antrea/pkg/client/clientset/versioned"
)

var (
	pluginName                           = "traceflowPlugin"
	addTfAction                          = "traceflowPlugin/addTf"
	showGraphAction                      = "traceflowPlugin/showGraphAction"
	client          *clientset.Clientset = nil
	kubeConfig                           = "KUBECONFIG"
)

const (
	tfNameCol       = "Trace"
	srcNamespaceCol = "Source Namespace"
	srcPodCol       = "Source Pod"
	dstNamespaceCol = "Destination Namespace"
	dstPodCol       = "Destination Pod"
	crdCol          = "Detailed Information"
)

// This is octant-trace-plugin.
// The plugin needs to run with octant version v0.10.2 or later.
func main() {
	localPlugin := newTraceflowPlugin()

	// Remove the prefix from the go logger since Octant will print logs with timestamps.
	log.SetPrefix("")

	capabilities := &plugin.Capabilities{
		ActionNames: []string{addTfAction, showGraphAction},
		IsModule:    true,
	}

	// Set up what should happen when Octant calls this plugin.
	options := []service.PluginOption{
		service.WithActionHandler(localPlugin.actionHandler),
		service.WithNavigation(localPlugin.navHandler, localPlugin.initRoutes),
	}

	p, err := service.Register(pluginName, "a description", capabilities, options...)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("octant-traceflow-plugin is starting")
	p.Serve()
}

type traceflowPlugin struct {
	client *clientset.Clientset
	graph  string
}

func newTraceflowPlugin() *traceflowPlugin {
	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv(kubeConfig))
	if err != nil {
		log.Fatalf("Failed to build kubeConfig %v", err)
	}
	client, err = clientset.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create K8s client for antrea-traceflow-octant-plugin %v", err)
	}
	return &traceflowPlugin{
		client: client,
		graph:  "",
	}
}

func (a *traceflowPlugin) navHandler(request *service.NavigationRequest) (navigation.Navigation, error) {
	return navigation.Navigation{
		Title:    "Trace Flow",
		Path:     request.GeneratePath("components"),
		IconName: "cloud",
	}, nil
}

func (a *traceflowPlugin) actionHandler(request *service.ActionRequest) error {
	actionName, err := request.Payload.String("action")
	if err != nil {
		return fmt.Errorf("unable to get input at string: %w", err)
	}

	switch actionName {
	case addTfAction:
		name, err := request.Payload.String("name")
		if err != nil {
			return fmt.Errorf("unable to get name at string : %w", err)
		}
		fromNamespace, err := request.Payload.String("fromNamespace")
		if err != nil {
			return fmt.Errorf("unable to get fromNamespace at string : %w", err)
		}
		fromPod, err := request.Payload.String("fromPod")
		if err != nil {
			return fmt.Errorf("unable to get fromPod at string : %w", err)
		}
		toNamespace, err := request.Payload.String("toNamespace")
		if err != nil {
			return fmt.Errorf("unable to get toNamespace at string : %w", err)
		}
		toPod, err := request.Payload.String("toPod")
		if err != nil {
			return fmt.Errorf("unable to get toPod at string : %w", err)
		}

		_, err = a.client.AntreaV1().Traceflows().Create(&v1.Traceflow{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			SrcNamespace: fromNamespace,
			SrcPod:       fromPod,
			DstNamespace: toNamespace,
			DstPod:       toPod,
			DstService:   "",
			RoundID:      "",
			Packet:       v1.Packet{},
			Status:       v1.Status{},
		})
		if err != nil {
			return err
		}
		return nil
	case showGraphAction:
		name, err := request.Payload.String("name")
		if err != nil {
			return fmt.Errorf("unable to get name at string : %w", err)
		}
		// Invoke GenGraph to show
		_, _ = a.client.AntreaV1().Traceflows().Get(name, metav1.GetOptions{})
		g := graphviz.New()
		graph, err := g.Graph()
		n, err := graph.CreateNode(name)
		if err != nil {
			log.Fatal(err)
		}
		m, err := graph.CreateNode("m")
		if err != nil {
			log.Fatal(err)
		}
		e, err := graph.CreateEdge("e", n, m)
		if err != nil {
			log.Fatal(err)
		}
		e.SetLabel("e")
		var buf bytes.Buffer
		if err := g.Render(graph, "dot", &buf); err != nil {
			log.Fatal(err)
		}
		a.graph = buf.String()
		if err := graph.Close(); err != nil {
			log.Fatal(err)
		}
		g.Close()
		return nil
	default:
		return fmt.Errorf("recieved action request for %s, but no handler defined", pluginName)
	}
}

func (a *traceflowPlugin) initRoutes(router *service.Router) {
	router.HandleFunc("/components", a.traceflowHandler)
}

func (a *traceflowPlugin) traceflowHandler(request *service.Request) (component.ContentResponse, error) {
	layout := flexlayout.New()

	card := component.NewCard("Antrea Traceflow")
	form := component.Form{Fields: []component.FormField{
		component.NewFormFieldText("name", "name", ""),
		component.NewFormFieldText("fromNamespace", "fromNamespace", ""),
		component.NewFormFieldText("fromPod", "fromPod", ""),
		component.NewFormFieldText("toNamespace", "toNamespace", ""),
		component.NewFormFieldText("toPod", "toPod", ""),
		component.NewFormFieldHidden("action", addTfAction),
	}}
	addTf := component.Action{
		Name:  "Start New Trace",
		Title: "Start New Trace",
		Form:  form,
	}
	graphForm := component.Form{Fields: []component.FormField{
		component.NewFormFieldText("name", "name", ""),
		component.NewFormFieldHidden("action", showGraphAction),
	}}
	genGraph := component.Action{
		Name:  "Generate Trace Graph",
		Title: "Generate Trace Graph",
		Form:  graphForm,
	}
	card.SetBody(component.NewText(""))
	card.AddAction(addTf)
	card.AddAction(genGraph)

	graphCard := component.NewCard("Antrea Traceflow Graph")
	if a.graph != "" {
		graphCard.SetBody(component.NewGraphviz(a.graph))
	} else {
		graphCard.SetBody(component.NewText(""))
	}
	listSection := layout.AddSection()
	err := listSection.Add(card, component.WidthFull)
	if err != nil {
		return component.ContentResponse{}, fmt.Errorf("error adding card to section: %w", err)
	}
	if a.graph != "" {
		err = listSection.Add(graphCard, component.WidthFull)
		if err != nil {
			return component.ContentResponse{}, fmt.Errorf("error adding graphCard to section: %w", err)
		}
	}

	tfCols := component.NewTableCols(tfNameCol, srcNamespaceCol, srcPodCol, dstNamespaceCol, dstPodCol, crdCol)
	tfTable := component.NewTableWithRows("Trace List", "", tfCols, a.getTfRows())
	return component.ContentResponse{
		Title: component.TitleFromString("Antrea Traceflow"),
		Components: []component.Component{
			layout.ToComponent("Antrea Traceflow"),
			tfTable,
		},
		IconName:   icon.Overview,
		IconSource: icon.Overview,
	}, nil
}

// getTfRows gets rows for displaying Controller information
func (a *traceflowPlugin) getTfRows() []component.TableRow {
	tfs, err := client.AntreaV1().Traceflows().List(metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to get Traceflows %v", err)
	}
	tfRows := make([]component.TableRow, 0)
	for _, tf := range tfs.Items {
		tfRows = append(tfRows, component.TableRow{
			tfNameCol:       component.NewText(tf.Name),
			srcNamespaceCol: component.NewText(tf.SrcNamespace),
			srcPodCol:       component.NewText(tf.SrcPod),
			dstNamespaceCol: component.NewText(tf.DstNamespace),
			dstPodCol:       component.NewText(tf.DstPod),
			crdCol: component.NewLink(tf.Name, tf.Name,
				"/cluster-overview/custom-resources/traceflows.antrea.tanzu.vmware.com/"+tf.Name),
		})
	}
	return tfRows
}