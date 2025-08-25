// Env package is meant to be used for loading config files
//
// Usage:
//
//	type Cfg struct {}
//	loader := env.NewLoader[*Cfg]()
//	loader.RegisterCallback(env.MustFn(env.FromYAMLConfigs[*Cfg]("config.yml")))
//	err = loader.Load()
//	if err != nil {
//		panic(err)
//	}
package env
