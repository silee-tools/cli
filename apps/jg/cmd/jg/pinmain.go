package main

// shouldPinMain 은 main working tree 를 피커 최상단에 고정할지 판정한다.
// main 경로가 비었거나 cwd 가 이미 main 루트와 같은 자리이면 고정하지 않는다.
// 수정 시 검토 관점: cwd 와 mainPath 는 표기(심볼릭 링크)가 어긋날 수 있으므로
// 반드시 canonicalPath 로 양쪽을 정규화한 뒤 비교한다. 한쪽만 정규화하면
// 같은 디렉토리를 다른 것으로 오판해 자기 자신을 고정 항목으로 띄운다.
func shouldPinMain(cwd, mainPath string) bool {
	if mainPath == "" {
		return false
	}
	return canonicalPath(cwd) != canonicalPath(mainPath)
}
