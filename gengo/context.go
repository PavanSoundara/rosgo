package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
)

// isRosPackage checks if a given path is a ROS package or not
func isRosPackage(dir string) bool {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, f := range files {
		if f.Name() == "package.xml" {
			return true
		}
	}
	return false
}

func findPackages(pkgType string, rosPkgPaths []string) (map[string]string, error) {
	pkgs := make(map[string]string)
	for _, p := range rosPkgPaths {
		files, err := ioutil.ReadDir(p)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !f.IsDir() {
				continue
			}
			pkgPath := filepath.Join(p, f.Name())
			if isRosPackage(pkgPath) {
				pkgName := filepath.Base(pkgPath)
				msgPath := filepath.Join(pkgPath, pkgType)
				msgPaths, err := filepath.Glob(msgPath + fmt.Sprintf("/*.%s", pkgType))
				if err != nil {
					continue
				}
				for _, m := range msgPaths {
					basename := filepath.Base(m)
					rootname := basename[:len(basename)-len(pkgType)-1]
					fullname := pkgName + "/" + rootname
					pkgs[fullname] = m
				}
			}
		}
	}
	return pkgs, nil
}

func findAllMessages(rosPkgPaths []string) (map[string]string, error) {
	return findPackages("msg", rosPkgPaths)
}

// findAllServices returns a path map with all ROS service definitons present in ROS package paths
// with keys as full service name including package and values as paths to the service definitions
func findAllServices(rosPkgPaths []string) (map[string]string, error) {
	return findPackages("srv", rosPkgPaths)
}

func findAllActions(rosPkgPaths []string) (map[string]string, error) {
	return findPackages("action", rosPkgPaths)
}

// MsgContext is used to load and create message or service specification from their definitions
// that are either passed explicitly or found in path map
type MsgContext struct {
	msgPathMap    map[string]string
	srvPathMap    map[string]string
	actionPathMap map[string]string
	msgRegistry   map[string]*MsgSpec
}

// NewMsgContext finds message/service definitions in the provided rosPkgPaths and
// initializes and returns new MsgContext with message and service path maps
// message and service path maps are used to find message definitions when loading
// a message without explicitly passing its definiton or path to its definition
func NewMsgContext(rosPkgPaths []string) (*MsgContext, error) {
	ctx := new(MsgContext)
	msgs, err := findAllMessages(rosPkgPaths)
	if err != nil {
		return nil, err
	}
	ctx.msgPathMap = msgs

	srvs, err := findAllServices(rosPkgPaths)
	if err != nil {
		return nil, err
	}
	ctx.srvPathMap = srvs

	acts, err := findAllActions(rosPkgPaths)
	if err != nil {
		return nil, err
	}
	ctx.actionPathMap = acts

	ctx.msgRegistry = make(map[string]*MsgSpec)
	return ctx, nil
}

// Register adds or updates a message specification in MsgContext's registery
func (ctx *MsgContext) Register(fullname string, spec *MsgSpec) {
	ctx.msgRegistry[fullname] = spec
}

// LoadMsgFromString loads a ROS message definition from a string and returns MsgSpec
// which contains extracted information from the definiton. Returns a non-nil error if
// an invalid string is passed or if failed to parse the message definition
func (ctx *MsgContext) LoadMsgFromString(text string, fullname string) (*MsgSpec, error) {
	packageName, shortName, e := packageResourceName(fullname)
	if e != nil {
		return nil, e
	}

	var fields []Field
	var constants []Constant
	for lineno, origLine := range strings.Split(text, "\n") {
		cleanLine := stripComment(origLine)
		if len(cleanLine) == 0 {
			// Skip empty line
			continue
		} else if strings.Contains(cleanLine, ConstChar) {
			constant, e := loadConstantLine(origLine)
			if e != nil {
				return nil, NewSyntaxError(fullname, lineno, e.Error())
			}
			constants = append(constants, *constant)
		} else {
			field, e := loadFieldLine(origLine, packageName)
			if e != nil {
				return nil, NewSyntaxError(fullname, lineno, e.Error())
			}
			fields = append(fields, *field)
		}
	}
	spec, _ := NewMsgSpec(fields, constants, text, fullname, OptionPackageName(packageName), OptionShortName(shortName))
	var err error
	md5sum, err := ctx.ComputeMsgMD5(spec)
	if err != nil {
		return nil, err
	}
	spec.MD5Sum = md5sum
	ctx.Register(fullname, spec)
	return spec, nil
}

// LoadMsgFromFile loads a ROS message definition from a file and returns MsgSpec
// which contains extracted information from the definiton. Returns a non-nil error if
// an invalid file path is passed or if failed to parse the message definition
func (ctx *MsgContext) LoadMsgFromFile(filePath string, fullname string) (*MsgSpec, error) {
	bytes, e := ioutil.ReadFile(filePath)
	if e != nil {
		return nil, e
	}
	text := string(bytes)
	return ctx.LoadMsgFromString(text, fullname)
}

// LoadMsg loads a message if the message definition is known from ROS package paths that
// was provided in the Constructor
func (ctx *MsgContext) LoadMsg(fullname string) (*MsgSpec, error) {
	if spec, ok := ctx.msgRegistry[fullname]; ok {
		return spec, nil
	}

	if path, ok := ctx.msgPathMap[fullname]; ok {
		spec, err := ctx.LoadMsgFromFile(path, fullname)
		if err != nil {
			return nil, err
		}

		ctx.msgRegistry[fullname] = spec
		return spec, nil
	}

	return nil, fmt.Errorf("Message definition of `%s` is not found", fullname)
}

// LoadSrvFromString loads a ROS service definition from a string and returns MsgSpec
// which contains extracted information from the definiton. Returns a non-nil error if
// an invalid string is passed or if failed to parse the service definition
func (ctx *MsgContext) LoadSrvFromString(text string, fullname string) (*SrvSpec, error) {
	packageName, shortName, err := packageResourceName(fullname)
	if err != nil {
		return nil, err
	}

	components := strings.Split(text, "---")
	if len(components) != 2 {
		return nil, fmt.Errorf("Syntax error: missing '---'")
	}

	reqText := components[0]
	resText := components[1]

	reqSpec, err := ctx.LoadMsgFromString(reqText, fullname+"Request")
	if err != nil {
		return nil, err
	}
	resSpec, err := ctx.LoadMsgFromString(resText, fullname+"Response")
	if err != nil {
		return nil, err
	}

	spec := &SrvSpec{
		packageName, shortName, fullname, text, "", reqSpec, resSpec,
	}
	md5sum, err := ctx.ComputeSrvMD5(spec)
	if err != nil {
		return nil, err
	}
	spec.MD5Sum = md5sum

	return spec, nil
}

// LoadSrvFromFile loads a ROS service definition from a file and returns MsgSpec
// which contains extracted information from the definiton. Returns a non-nil error if
// an invalid file path is passed or if failed to parse the service definition
func (ctx *MsgContext) LoadSrvFromFile(filePath string, fullname string) (*SrvSpec, error) {
	bytes, e := ioutil.ReadFile(filePath)
	if e != nil {
		return nil, e
	}
	text := string(bytes)
	return ctx.LoadSrvFromString(text, fullname)
}

// LoadSrv loads a service if the service definition is known from ROS package paths that
// was provided in the Constructor
func (ctx *MsgContext) LoadSrv(fullname string) (*SrvSpec, error) {
	if path, ok := ctx.srvPathMap[fullname]; ok {
		spec, err := ctx.LoadSrvFromFile(path, fullname)
		if err != nil {
			return nil, err
		}
		return spec, nil
	}
	return nil, fmt.Errorf("Service definition of `%s` is not found", fullname)
}

// LoadActionFromString loads a ROS action definition from a string and returns MsgSpec
// which contains extracted information from the definiton. Returns a non-nil error if
// an invalid string is passed or if failed to parse the service definition
func (ctx *MsgContext) LoadActionFromString(text string, fullname string) (*ActionSpec, error) {
	packageName, shortName, err := packageResourceName(fullname)
	if err != nil {
		return nil, err
	}

	components := strings.Split(text, "---")
	if len(components) != 3 {
		return nil, fmt.Errorf("Syntax error: missing '---'")
	}

	goalText := components[0]
	resultText := components[1]
	feedbackText := components[2]
	goalSpec, err := ctx.LoadMsgFromString(goalText, fullname+"Goal")
	if err != nil {
		return nil, err
	}
	actionGoalText := "Header header\nactionlib_msgs/GoalID goal_id\n" + fullname + "Goal goal\n"
	actionGoalSpec, err := ctx.LoadMsgFromString(actionGoalText, fullname+"ActionGoal")
	if err != nil {
		return nil, err
	}
	feedbackSpec, err := ctx.LoadMsgFromString(feedbackText, fullname+"Feedback")
	if err != nil {
		return nil, err
	}
	actionFeedbackText := "Header header\nactionlib_msgs/GoalStatus status\n" + fullname + "Feedback feedback"
	actionFeedbackSpec, err := ctx.LoadMsgFromString(actionFeedbackText, fullname+"ActionFeedback")
	if err != nil {
		return nil, err
	}
	resultSpec, err := ctx.LoadMsgFromString(resultText, fullname+"Result")
	if err != nil {
		return nil, err
	}
	actionResultText := "Header header\nactionlib_msgs/GoalStatus status\n" + fullname + "Result result"
	actionResultSpec, err := ctx.LoadMsgFromString(actionResultText, fullname+"ActionResult")
	if err != nil {
		return nil, err
	}

	spec := &ActionSpec{
		Package:        packageName,
		ShortName:      shortName,
		FullName:       fullname,
		Text:           text,
		Goal:           goalSpec,
		Feedback:       feedbackSpec,
		Result:         resultSpec,
		ActionGoal:     actionGoalSpec,
		ActionFeedback: actionFeedbackSpec,
		ActionResult:   actionResultSpec,
	}

	md5sum, err := ctx.ComputeActionMD5(spec)
	if err != nil {
		return nil, err
	}
	spec.MD5Sum = md5sum
	return spec, nil
}

// LoadActionFromFile loads a ROS action definition from a file and returns MsgSpec
// which contains extracted information from the definiton. Returns a non-nil error if
// an invalid file path is passed or if failed to parse the service definition
func (ctx *MsgContext) LoadActionFromFile(filePath string, fullname string) (*ActionSpec, error) {
	bytes, e := ioutil.ReadFile(filePath)
	if e != nil {
		return nil, e
	}
	text := string(bytes)
	return ctx.LoadActionFromString(text, fullname)
}

// LoadAction loads an  if the action definition is known from ROS package paths that
// was provided in the Constructor
func (ctx *MsgContext) LoadAction(fullname string) (*ActionSpec, error) {
	if path, ok := ctx.actionPathMap[fullname]; ok {
		spec, err := ctx.LoadActionFromFile(path, fullname)
		if err != nil {
			return nil, err
		}
		return spec, nil
	}
	return nil, fmt.Errorf("Action definition of `%s` is not found", fullname)
}

// ComputeMD5Text computes the MD5 hash of given *MsgSpec
func (ctx *MsgContext) ComputeMD5Text(spec *MsgSpec) (string, error) {
	var buf bytes.Buffer
	for _, c := range spec.Constants {
		buf.WriteString(fmt.Sprintf("%s %s=%s\n", c.Type, c.Name, c.ValueText))
	}
	for _, f := range spec.Fields {
		if f.Package == "" {
			buf.WriteString(fmt.Sprintf("%s\n", f.String()))
		} else {
			subspec, err := ctx.LoadMsg(f.Package + "/" + f.Type)
			if err != nil {
				return "", nil
			}
			submd5, err := ctx.ComputeMsgMD5(subspec)
			if err != nil {
				return "", nil
			}
			buf.WriteString(fmt.Sprintf("%s %s\n", submd5, f.Name))
		}
	}
	return strings.Trim(buf.String(), "\n"), nil
}

// ComputeMsgMD5 computes the MD5 hash of a ROS message from its MsgSpec
func (ctx *MsgContext) ComputeMsgMD5(spec *MsgSpec) (string, error) {
	md5text, err := ctx.ComputeMD5Text(spec)
	if err != nil {
		return "", err
	}
	hash := md5.New()
	hash.Write([]byte(md5text))
	sum := hash.Sum(nil)
	md5sum := hex.EncodeToString(sum)
	return md5sum, nil
}

// ComputeActionMD5 computes the MD5 hash of a ROS action from its MsgSpec
func (ctx *MsgContext) ComputeActionMD5(spec *ActionSpec) (string, error) {
	goalText, err := ctx.ComputeMD5Text(spec.ActionGoal)
	if err != nil {
		return "", err
	}
	feedbackText, err := ctx.ComputeMD5Text(spec.ActionFeedback)
	if err != nil {
		return "", err
	}
	resultText, err := ctx.ComputeMD5Text(spec.ActionResult)
	if err != nil {
		return "", err
	}
	hash := md5.New()
	hash.Write([]byte(goalText))
	hash.Write([]byte(feedbackText))
	hash.Write([]byte(resultText))
	sum := hash.Sum(nil)
	md5sum := hex.EncodeToString(sum)
	return md5sum, nil
}

// ComputeSrvMD5 computes the MD5 hash of a ROS service from its MsgSpec
func (ctx *MsgContext) ComputeSrvMD5(spec *SrvSpec) (string, error) {
	reqText, err := ctx.ComputeMD5Text(spec.Request)
	if err != nil {
		return "", err
	}
	resText, err := ctx.ComputeMD5Text(spec.Response)
	if err != nil {
		return "", err
	}
	hash := md5.New()
	hash.Write([]byte(reqText))
	hash.Write([]byte(resText))
	sum := hash.Sum(nil)
	md5sum := hex.EncodeToString(sum)
	return md5sum, nil
}
