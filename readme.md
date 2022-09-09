[Server Example](https://github.com/MeteorsLiu/TCPConnectionPoolServer)

[Client Dial using connections pool Example](https://github.com/MeteorsLiu/go-tcpConnectionPool/blob/main/netconn_test.go)

# 这个是什么

这个是一个**客户端**的连接池。其原理就是建立一个简单的**线程安全**队列，提前建立好MIN_SIZE个连接，当调用Read()或者Write()函数的时候，会从队列中取一个，然后写入或者读取，然后再放回队列中，方便下一次复用。只有当你调用Close()函数，建立的连接才能被安全的关闭。

# 为什么要这么设计

1.允许在不同goroutine中对TCP连接进行Read，Write。也就是说，你不必再拘束于一个TCP连接，你可以同时写入多个或者读取多个，而且这是线程安全的。（为什么？因为每个连接都是独立的，互不影响）

2.上述并发读写很美妙，但是会出现一个问题，当goroutine太多，超过系统HARD LIMIT/SOFT LIMIT时候，触发Too many open files等错误，为解决此问题，连接池会限制最大连接数，当池子中已经没有可用连接，会等候连接入队进行重利用，当超过TIMEOUT时间后，仍没有入队连接，会尝试建立连接。

