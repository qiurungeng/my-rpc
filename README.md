# MyRPC

## 实现一个RPC框架应该关注什么问题？   

两个应用程序互相调用，应该采用什么传输协议？   

- 如果这个两个应用程序位于不同的机器，那么一般会选择 TCP 协议或者 HTTP 协议
- 如果两个应用程序位于相同的机器，也可以选择 Unix Socket 协议

确定报文的编码格式：

- JSON, XML, Protobuf...

可用性问题：

- 连接超时，是否支持异步请求和并发？

分布式问题：

如果服务端的实例很多，客户端并不关心这些实例的地址和部署位置，只关心自己能否获取到期待的结果，那就引出了注册中心(registry)和负载均衡(load balance)的问题。

- 注册中心：   
  客户端和服务端互相不感知对方的存在，服务端启动时将自己注册到注册中心，客户端调用时，从注册中心获取到所有可用的实例，选择一个来调用。这样服务端和客户端只需要感知注册中心的存在就够了。注册中心通常还需要实现服务动态添加、删除，使用心跳确保服务处于可用状态等功能。
  
- 负载均衡：
  假设有多个服务实例，每个实例提供相同的功能，为了提高整个系统的吞吐量，每个实例部署在不同的机器上。客户端可以选择任意一个实例进行调用，获取想要的结果。那如何选择呢？取决了负载均衡的策略
  
 