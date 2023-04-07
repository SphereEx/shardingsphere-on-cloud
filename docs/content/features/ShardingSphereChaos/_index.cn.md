### ShardingSphereChaos说明文档

#### 配置示例

```yaml
apiVersion: shardingsphere.apache.org/v1alpha1
kind: ShardingSphereChaos
metadata:
  labels:
    app.kubernetes.io/name: shardingsphereChaos
  name: shardingspherechaos-lala
  namespace: default
  annotations:
    selector.chaos-mesh.org/mode: "all"
spec:
  chaosKind: podChaos
  podChaos:
    selector:
      labelSelectors:
        app.kubernetes.io/component: zookeeper-new
      namespaces: [ "mesh-test" ]
    params:
      podFailure:
        duration: 2m
    action: "podFailure"
```

#### spec

* 注入故障
  `.spec.chaosKind`用于指定注入故障的类型
  在spec中配置的是故障的通用字段,在接入平台提供的故障时,需要在annoations里面写入平台类型,并将针对于这个平台的,故障spec中没有提到的字段,写入annoations中

    * 通用配置字段

        * Selector
          故障目标选择器,它以`(,inline)`的形式在chaos的spec配置


      | namespaces          | 指定命名空间               |
      | --------------------- | ---------------------------- |
      | labelSelectors      | 指定选择标签               |
      | annotationSelectors | 指定注释                   |
      | nodes               | 指定节点                   |
      | pods                | 以命名空间-pod名的方式指定 |
      | nodeSelectors       | 以label和node来选择节点    |
    * PodChaosSpec

    这部分声明在.`spec.podChaos`中
    定义pod类型的故障,action字段声明了注入pod故障的类型


    | action                       | 指定pod的故障类型,分为podFailure,containerKill |
    | ------------------------------ | ------------------------------------------------ |
    | podFailure.Duration          | 指定PodFailureAction的生效时间                 |
    | containerKill.containerNames | 指定要杀掉的容器                               |
* networkSpec
  这部分声明在`.spec.networkChaos`中

    1. 定义network类型的故障


    | Action                                                       | 指定network的故障类型,分为delay,duplicate,corrupt,partition,loss                                            |
    | -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
    | Duration                                                     | 指定故障的持续时长                                                                                          |
    | Direction                                                    | 用于指定网络故障的方向,在不指定时默认为to,分为to(->target),from(target<-),both(<->target)                   |
    | target                                                       | selector选择器,用于选择目标对象                                                                             |
    | Source                                                       | selector选择器,用于选择起始对象                                                                             |
    | delay.latency<br /><br />delay.correlation<br />delay.jitter | latency: 指示网络延迟<br />correlation: 当前延迟与上一延迟之间的相关性jitter: 表示网络延迟的范围<br />      |
    |                                                              |                                                                                                             |
    | loss.correlation<br />loss.loss                              | loss: 丢包的概率<br />correlation: 表示当前丢包概率与上次丢包概率之间的相关性                               |
    |                                                              |                                                                                                             |
    | duplicate.correlation<br />duplicate.duplicate               | correlation: 指示当前报文复制概率之间的相关性<br />duplicate: 指示数据包复制的概率                          |
    |                                                              |                                                                                                             |
    | corrupt.corrupt<br />corrupt.correlation                     | corrupt: 指示数据包损坏的概率<br />correlation:  指示当前数据包损坏概率与上一次数据包损坏概率之间的相关性。 |
    |                                                              |                                                                                                             |

    * 特定配置字段
      这部分需要声明在annoations里或env中

      * chaos-mesh


        | podchaos的配置字段<br/>     | spec/mode   <----->      selector.mode<br />spec/value    <----->       selector.value<br />spec/pod/action <----->   specify .action<br />spec/pod/gracePeriod     <-----> specify .gracePeriod                                                                                                                                                                                                                                                                               |
        | ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
        | networkchaos的配置字段<br/> | spec/device  <-----> .device<br />spec/targetDevice <-----> .targetDevice<br />spec/target/mode <-----> .selector.mode<br />spec/target/value <-----> .value<br />spec/network/action <-----> specify .action<br />spec/network/rate <-----> .bandwidth.rate<br />spec/network/limit <-----> .bandwidth.limit<br />spec/network/buffer <-----> .bandwidth.buffer<br />spec/network/peakrate <-----> .bandwidth.peakrate<br />spec/network/minburst <-----> .bandwidth.minburst |
        |                             |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
      * Litmus chaos


        | podchaos的配置字段     | * pod-delete<br />spec/random  <-------> RANDOMNESS  <br />spec/force <-----> FORCE<br />* Container-kill  <br />spec/signal <------> SIGNAL  <br />spec/chaos_interval <-----> CHAOS_INTERVAL                                       |
        | ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
        | networkchaos的配置字段 | <br/>                                                                                                                                                                                                                                |
        | 公共配置字段           | spec/action <----> .spec.experiments.name<br />spec/ramp_time <-----> RAMP_TIME<br />spec/duration <-------> TOTAL_CHAOS_DURATION<br />spec/sequence <-----> SEQUENCE<br />spec/lib_image <-----> LIB_IMAGE<br />spec/lib <----> LIB |
        |                        |                                                                                                                                                                                                                                      |

### Status


* DeploymentCondition
  该字段记录了注入故障的进度,它有以下四个阶段


    | Creating     | 代表chaos在创建阶段,还没完成注入                            |
    | -------------- | ------------------------------------------------------------- |
    | AllRecovered | 代表环境已从故障中恢复                                      |
    | Paused       | 实验暂停,可能是因为选择的节点不存在,考虑是否crd的定义有问题     |
    | AllInjected  | 该阶段代表故障已经成功注入到环境中                           |
    |              |                                                          |
